package dawproject
import(
 "path/filepath"
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
