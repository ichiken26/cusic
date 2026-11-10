package main

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

func fixtureSourceDAWProject(t *testing.T,output string) {
 t.Helper()
 f,err:=os.Create(output);if err!=nil{t.Fatal(err)}
 zipWriter:=zip.NewWriter(f)
 data:=`<Project version="1.0"><Transport><Tempo value="168"/><TimeSignature numerator="4" denominator="4"/></Transport><Structure>
<Track name="Bass" id="bass" contentType="notes" loaded="true"><Channel role="regular" id="channel_bass" audioChannels="2">
<Devices><Vst3Plugin id="kontakt" deviceID="kontakt-uuid" deviceName="Kontakt" deviceVendor="Native Instruments" deviceRole="instrument"><State path="plugins/kontakt.vstpreset" external="false"/></Vst3Plugin>
<Vst3Plugin id="comp" deviceID="compressor-uuid" deviceName="My Compressor" deviceRole="audioFX"><State path="plugins/comp.vstpreset" external="false"/></Vst3Plugin></Devices>
<Pan value="0.5"/><Volume value="1"/></Channel></Track></Structure>
<Arrangement><Lanes id="timeline" timeUnit="beats"><Lanes id="basslane" track="bass">
<Clips id="bassclips"><Clip time="0" duration="2" playStart="0"><Notes id="notes"><Note time="0" duration="1" channel="0" key="40" vel="0.7874015748" rel="0.8"/></Notes></Clip></Clips>
</Lanes></Lanes></Arrangement></Project>`
 for name,b:=range map[string][]byte{"project.xml":[]byte(data),"plugins/kontakt.vstpreset":{0,2,255,3},"plugins/comp.vstpreset":{0,7,99,0,1}}{
  w,e:=zipWriter.Create(name);if e!=nil{t.Fatal(e)}
  if _,e=w.Write(b);e!=nil{t.Fatal(e)}
 }
 if err=zipWriter.Close();err!=nil{t.Fatal(err)}
 if err=f.Close();err!=nil{t.Fatal(err)}
}
func TestCLIImportExportPreservesDAWProjectEffects(t *testing.T){
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 root:=t.TempDir()
 if err=os.Chdir(root);err!=nil{t.Fatal(err)}
 defer os.Chdir(cwd)
 fixtureSourceDAWProject(t,"original.dawproject")
 if err=run([]string{"init","--name","My Song","--tempo","168"});err!=nil{t.Fatal(err)}
 if err=run([]string{"import","original.dawproject"});err!=nil{t.Fatal(err)}
 if err=run([]string{"source","status"});err!=nil{t.Fatal(err)}
 if err=run([]string{"export","--format","dawproject","--out","original-preserved.dawproject"});err!=nil{t.Fatal(err)}
 original,err:=os.ReadFile("original.dawproject");if err!=nil{t.Fatal(err)}
 preserved,err:=os.ReadFile("original-preserved.dawproject");if err!=nil{t.Fatal(err)}
 if !bytes.Equal(original,preserved){t.Fatal("plugins/settings were re-serialized when notes are unchanged")}
 s,err:=store.Open(root);if err!=nil{t.Fatal(err)}
 revisions,err:=s.Revisions(1);if err!=nil{t.Fatal(err)}
 if len(revisions)==0{t.Fatal("import revision missing")}
 if _,err=s.ApplyPatch(store.Patch{BaseRevision:revisions[0].ID,Track:"Bass",Message:"edit bass",FromBar:1,ToBar:1,Notes:[]store.InputNote{{Tick:0,Duration:240,Pitch:43,Velocity:110}}});err!=nil{t.Fatal(err)}
 s.Close()
 if err=run([]string{"export","--format","dawproject","--out","edited-preserved.dawproject"});err!=nil{t.Fatal(err)}
 if err=run([]string{"effects","inspect","--from","edited-preserved.dawproject"});err!=nil{t.Fatal(err)}
 zipFile,err:=zip.OpenReader("edited-preserved.dawproject");if err!=nil{t.Fatal(err)}
 defer zipFile.Close()
 foundState:=0
 for _,part:=range zipFile.File{
  if strings.HasPrefix(part.Name,"plugins/"){foundState++}
  if part.Name=="project.xml"{
   source,err:=part.Open();if err!=nil{t.Fatal(err)}
   b,err:=io.ReadAll(source);source.Close();if err!=nil{t.Fatal(err)}
   if !strings.Contains(string(b),`key="43"`)||!strings.Contains(string(b),"Kontakt"){t.Fatal("patch lost notes or Kontakt state")}
  }
 }
 if foundState!=2{t.Fatalf("expected original VST state files, found %d",foundState)}
 if _,err=os.Stat(filepath.Join(root,".cusic","dawproject-source.json"));err!=nil{t.Fatal(err)}
}
