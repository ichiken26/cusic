package dawproject
import(
 "path/filepath"
 "archive/zip"
 "os"
 "testing"
 "github.com/ichiken26/cusic/internal/store"
)
func TestNoteRoundTrip(t *testing.T){
 file:=filepath.Join(t.TempDir(),"demo.dawproject")
 settings:=store.Settings{Name:"Demo",Tempo:168,MeterNum:4,MeterDen:4,PPQ:480}
 tracks:=[]store.Track{{ID:"bassid",Name:"Bass",Kind:"instrument"}}
 notes:=map[string][]store.Note{"bassid":{{ID:"a",TrackID:"bassid",Tick:1920,Duration:480,Pitch:40,Velocity:100}}}
 if err:=WriteFile(file,settings,tracks,notes,nil);err!=nil{t.Fatal(err)}
 song,err:=ReadFile(file);if err!=nil{t.Fatal(err)}
 if song.Song.Tempo!=168||len(song.Song.Tracks)!=1{t.Fatalf("%#v",song)}
 n:=song.Song.Tracks[0].Notes[0]
 if n.Tick!=1920||n.Duration!=480||n.Pitch!=40||n.Velocity!=100{t.Fatalf("%#v",n)}
}

func TestNestedCubaseNoteLanes(t *testing.T){
 filename:=filepath.Join(t.TempDir(),"nested.dawproject")
 f,err:=os.Create(filename);if err!=nil{t.Fatal(err)}
 z:=zip.NewWriter(f);w,err:=z.Create("project.xml");if err!=nil{t.Fatal(err)}
 xmlDoc:=`<Project version="1.0"><Application name="Cubase" version="15"/><Transport><Tempo value="168"/><TimeSignature numerator="4" denominator="4"/></Transport><Structure><Track id="folder" contentType="audio notes" name="Band"><Track id="bass" contentType="notes" name="Bass"><Channel id="c" role="regular"/></Track></Track></Structure><Arrangement><Lanes><Lanes><Lanes track="bass"><Clips><Clip time="2" duration="1" playStart="0"><Notes><Note time="0" duration="1" channel="0" key="40" vel="0.7874"/></Notes></Clip></Clips></Lanes></Lanes></Lanes></Arrangement></Project>`
 if _,err=w.Write([]byte(xmlDoc));err!=nil{t.Fatal(err)}
 if err=z.Close();err!=nil{t.Fatal(err)};if err=f.Close();err!=nil{t.Fatal(err)}
 data,err:=ReadFile(filename);if err!=nil{t.Fatal(err)}
 if len(data.Song.Tracks)!=1||data.Song.Tracks[0].Name!="Bass"{t.Fatalf("missing nested bass: %#v",data)}
 note:=data.Song.Tracks[0].Notes[0]
 if note.Tick!=960||note.Duration!=480||note.Pitch!=40{t.Fatalf("incorrect nested clip timing: %#v",note)}
}
