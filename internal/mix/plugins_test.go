package mix

import (
 "archive/zip"
 "bytes"
 "io"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/ichiken26/cusic/internal/store"
)

func createPluginFixture(t *testing.T,advanced bool)string {
 t.Helper()
 file:=filepath.Join(t.TempDir(),"original.dawproject")
 out,err:=os.Create(file);if err!=nil{t.Fatal(err)}
 z:=zip.NewWriter(out)
 xml:=`<Project version="1.0"><Structure>
<Track id="bass" name="Bass (Kontakt)" contentType="notes" loaded="true">
 <Channel audioChannels="2" role="regular" destination="master">
 <Devices><Vst3Plugin id="kontakt" deviceID="instrument-uuid" deviceName="Kontakt" deviceVendor="Native Instruments" deviceRole="instrument"><State path="plugins/kontakt.vstpreset" external="false"/></Vst3Plugin>
 <Vst3Plugin id="fx" deviceID="fx-uuid" deviceName="Molotok" deviceVendor="TDR" deviceRole="audioFX"><State path="plugins/fx.vstpreset" external="false"/></Vst3Plugin></Devices>
 <Pan value="0.1"/><Volume value="1"/>
 </Channel></Track>
<Track id="guitar" name="Guitar" contentType="notes"><Channel audioChannels="2" role="regular" destination="master"><Pan value="0.5"/></Channel></Track>
</Structure><Arrangement><Lanes timeUnit="beats">
<Lanes track="bass"><Clips><Clip time="0" duration="2" playStart="0"><Notes id="bass-notes"><Note time="0" duration="1" key="40" vel="0.8" rel="0.8" channel="0">`
 if advanced{xml+=`<Expression><Point time="0" value="0.1"/></Expression>`}
 xml+=`</Note></Notes></Clip></Clips></Lanes></Lanes></Arrangement></Project>`
 for name,data:=range map[string][]byte{
  "project.xml":[]byte(xml),
  "plugins/kontakt.vstpreset":{0,1,0xfe,0xfd},
  "plugins/fx.vstpreset":{8,0xff,0x20,0x4a},
  "audio/stem.wav":{0x52,0x49,0x46,0x46},
 }{
  writer,e:=z.Create(name);if e!=nil{t.Fatal(e)}
  if _,e=writer.Write(data);e!=nil{t.Fatal(e)}
 }
 if err=z.Close();err!=nil{t.Fatal(err)}
 if err=out.Close();err!=nil{t.Fatal(err)}
 return file
}
func readZipMember(t *testing.T,file,name string)[]byte{
 t.Helper()
 r,err:=zip.OpenReader(file);if err!=nil{t.Fatal(err)}
 defer r.Close()
 for _,z:=range r.File{if z.Name==name{
  f,e:=z.Open();if e!=nil{t.Fatal(e)}
  b,e:=io.ReadAll(f);f.Close();if e!=nil{t.Fatal(e)}
  return b
 }}
 t.Fatalf("%s missing in %s",name,file)
 return nil
}
func TestEffectsInspectAndGenericCompressor(t *testing.T){
 src:=createPluginFixture(t,false)
 report,err:=InspectDevices(src);if err!=nil{t.Fatal(err)}
 if len(report.Kontakt)!=1||report.Kontakt[0].AudioChannels!=2||len(report.Kontakt[0].Plugins)!=2{
  t.Fatalf("incorrect Kontakt/plug-in discovery: %+v",report)
 }
 if len(report.Warnings)==0{t.Fatal("Kontakt output diagnostic warning missing")}
 plan:=DevicePlan{Operations:[]DeviceOperation{{Track:"Guitar",Action:"insertCompressor",Compressor:&CompressorSettings{
  AttackMS:12,ReleaseMS:115,RatioPercent:50,ThresholdDB:-16,OutputGainDB:2,
 }}}}
 if err=CheckDevices(src,plan);err!=nil{t.Fatal(err)}
 out:=filepath.Join(t.TempDir(),"with-comp.dawproject")
 if err=ApplyDevices(src,out,plan);err!=nil{t.Fatal(err)}
 project:=string(readZipMember(t,out,"project.xml"))
 if !strings.Contains(project,`<Compressor`)||!strings.Contains(project,`unit="seconds"`)||!strings.Contains(project,`unit="percent"`){
  t.Fatalf("generic compressor schema incorrectly exported: %s",project)
 }
 for _,member:=range []string{"plugins/fx.vstpreset","plugins/kontakt.vstpreset","audio/stem.wav"}{
  if !bytes.Equal(readZipMember(t,src,member),readZipMember(t,out,member)){t.Fatalf("%s changed",member)}
 }
 if err=ApplyDevices(src,out,plan);err==nil{t.Fatal("existing destination overwritten")}
}
func TestCloneVst3StateAndProtectOpaqueParameters(t *testing.T){
 src:=createPluginFixture(t,false)
 plan:=DevicePlan{Operations:[]DeviceOperation{{Action:"cloneVst3",Track:"Guitar",FromTrack:"Bass (Kontakt)",FromPluginID:"fx"}}}
 if err:=CheckDevices(src,plan);err!=nil{t.Fatal(err)}
 out:=filepath.Join(t.TempDir(),"clone.dawproject")
 if err:=ApplyDevices(src,out,plan);err!=nil{t.Fatal(err)}
 report,err:=InspectDevices(out);if err!=nil{t.Fatal(err)}
 var cloned DeviceInfo
 for _,tinfo:=range report.Tracks{if tinfo.Track=="Guitar"&&len(tinfo.Plugins)>0{cloned=tinfo.Plugins[0]}}
 if cloned.StatePath==""||!cloned.StatePresent{t.Fatalf("cloned plugin state missing: %+v",cloned)}
 if !bytes.Equal(readZipMember(t,out,cloned.StatePath),readZipMember(t,src,"plugins/fx.vstpreset")){t.Fatal("cloned state payload changed")}
 invalid:=DevicePlan{Operations:[]DeviceOperation{{Action:"setParameters",Track:"Bass (Kontakt)",PluginID:"fx",Parameters:map[string]float64{"threshold":-8}}}}
 if err=CheckDevices(src,invalid);err==nil{t.Fatal("opaque state was edited as fake XML parameters")}
}
func TestPreservedExportKeepsAllOriginalState(t *testing.T){
 src:=createPluginFixture(t,false)
 dest:=filepath.Join(t.TempDir(),"untouched.dawproject")
 if err:=ExportPreserving(src,dest,nil);err!=nil{t.Fatal(err)}
 original,e:=os.ReadFile(src);if e!=nil{t.Fatal(e)}
 after,e:=os.ReadFile(dest);if e!=nil{t.Fatal(e)}
 if !bytes.Equal(original,after){t.Fatal("unchanged DAWproject was not copied byte-for-byte")}
 changed:=map[string][]store.Note{"Bass (Kontakt)":{{Tick:0,Duration:240,Pitch:43,Velocity:100}}}
 newFile:=filepath.Join(t.TempDir(),"notes-edited.dawproject")
 if err:=ExportPreserving(src,newFile,changed);err!=nil{t.Fatal(err)}
 for _,member:=range []string{"plugins/fx.vstpreset","plugins/kontakt.vstpreset","audio/stem.wav"}{
  if !bytes.Equal(readZipMember(t,src,member),readZipMember(t,newFile,member)){t.Fatalf("changed original asset %s",member)}
 }
 project:=string(readZipMember(t,newFile,"project.xml"))
 if !strings.Contains(project,`key="43"`)||!strings.Contains(project,"Kontakt")||!strings.Contains(project,"Molotok"){
  t.Fatal("edited notes or plugin metadata missing")
 }
}
func TestPreservedExportRejectsExpressionsBeforeWriting(t *testing.T){
 src:=createPluginFixture(t,true)
 dest:=filepath.Join(t.TempDir(),"out.dawproject")
 changed:=map[string][]store.Note{"Bass (Kontakt)":{{Tick:0,Duration:480,Pitch:41,Velocity:98}}}
 if err:=ExportPreserving(src,dest,changed);err==nil{t.Fatal("complex note expression should refuse flattening")}
 if _,err:=os.Stat(dest);!os.IsNotExist(err){t.Fatal("output was written despite refusal")}
}
