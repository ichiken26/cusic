package mix
import(
 "archive/zip"
 "os"
 "path/filepath"
 "bytes"
 "math"
 "io"
 "strconv"
 "testing"
)
func TestApplyPreservesOtherTracksAndPluginState(t *testing.T){
 folder:=t.TempDir();src:=filepath.Join(folder,"original.dawproject");out:=filepath.Join(folder,"mixed.dawproject")
 f,err:=os.Create(src);if err!=nil{t.Fatal(err)}
 z:=zip.NewWriter(f)
 x,_:=z.Create("project.xml")
 project:=`<Project version="1.0"><Structure><Track name="Bass" id="bass"><Channel><Devices><Vst3Plugin id="plugin" name="Amp"/></Devices><Pan unit="normalized" value="0.5"/><Volume unit="linear" value="1"/></Channel></Track><Track name="Miku" id="miku"><Channel><Pan unit="normalized" value="0.5"/><Volume unit="linear" value="1"/></Channel></Track></Structure></Project>`
 x.Write([]byte(project));bin,_:=z.Create("plugins/state.bin");bin.Write([]byte{0,0xff,0xfe,8})
 z.Close();f.Close()
 db:=-6.0;pan:=0.4
 plan:=Plan{Tracks:[]Track{{Name:"Bass",FaderDB:&db,Pan:&pan,EQ:[]Band{{Type:"bell",Freq:180,Gain:-3,Q:1.2}}}}}
 if err=Apply(src,out,plan);err!=nil{t.Fatal(err)}
 archive,err:=zip.OpenReader(out);if err!=nil{t.Fatal(err)};defer archive.Close()
 var body []byte
 for _,file:=range archive.File{
  in,e:=file.Open();if e!=nil{t.Fatal(e)}
  content,_:=io.ReadAll(in);in.Close()
  if file.Name=="project.xml"{body=content}
  if file.Name=="plugins/state.bin"&&!bytes.Equal(content,[]byte{0,0xff,0xfe,8}){t.Fatal("plugin state changed")}
 }
 p,e:=parse(body);if e!=nil{t.Fatal(e)}
 structure:=p.child("Structure")
 tracks:=[]*node{}
 for _,part:=range structure.Children{if node,ok:=part.(*node);ok&&node.Name.Local=="Track"{tracks=append(tracks,node)}}
 bass:=tracks[0].child("Channel");miku:=tracks[1].child("Channel")
 if bass.child("Devices").child("Equalizer")==nil{t.Fatal("no EQ")}
 if bass.child("Devices").child("Vst3Plugin")==nil{t.Fatal("original VST missing")}
 v,e:=strconv.ParseFloat(bass.child("Volume").attr("value"),64)
 if e!=nil||math.Abs(v-0.501187)>0.001{t.Fatalf("invalid volume %f %v",v,e)}
 if miku.child("Volume").attr("value")!="1"{t.Fatal("modified unrelated track")}
 if err:=Apply(src,out,plan);err==nil{t.Fatal("overwrote existing mix")}
}
