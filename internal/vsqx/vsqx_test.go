package vsqx
import(
 "bytes"
 "testing"
 "github.com/ichiken26/cusic/internal/store"
)
func TestExportWithVoicebankTemplate(t *testing.T){
 template:=[]byte(`<?xml version="1.0"?><vsq4 xmlns="http://www.yamaha.co.jp/vocaloid/schema/vsq4/"><vVoiceTable><vVoice><id>MY_V4X_BANK</id></vVoice></vVoiceTable><masterTrack><seqName>template</seqName><resolution>480</resolution><preMeasure>4</preMeasure><timeSig><m>0</m><nu>4</nu><de>4</de></timeSig><tempo><t>0</t><v>12000</v></tempo></masterTrack><vsTrack><name>Miku</name><vsPart><t>7680</t><playTime>1920</playTime><singer><t>0</t></singer><note><t>0</t><dur>240</dur><n>55</n></note></vsPart></vsTrack></vsq4>`)
 track:=store.Track{ID:"t",Name:"Miku",Kind:"vocal"}
 notes:=[]store.Note{{ID:"n",TrackID:"t",Tick:0,Duration:480,Pitch:69,Velocity:100,Lyric:"あ"}}
 data,err:=Export(template,store.Settings{Name:"Demo",Tempo:168,MeterNum:4,MeterDen:4,PPQ:480},track,notes)
 if err!=nil{t.Fatal(err)}
 if !bytes.Contains(data,[]byte("MY_V4X_BANK"))||!bytes.Contains(data,[]byte("<y>あ</y>")) {t.Fatalf("missing voice/lyrics: %s",data)}
 root,err:=parse(data);if err!=nil||root.Name.Local!="vsq4"{t.Fatalf("invalid output %v",err)}
}

func TestPreserveTuningWhenEditingExisting(t *testing.T){
 old:=[]byte(`<?xml version="1.0"?><vsq4 xmlns="http://www.yamaha.co.jp/vocaloid/schema/vsq4/"><vVoiceTable><vVoice><id>MIKU</id></vVoice></vVoiceTable><masterTrack><preMeasure>4</preMeasure></masterTrack><vsTrack><name>Miku</name><vsPart><t>7680</t><mCtrl><t>20</t><v id="PIT">4500</v></mCtrl><note><t>0</t><dur>480</dur><n>69</n><y>あ</y><p>a</p><nStyle><v id="vibLen">65</v></nStyle></note><note><t>480</t><dur>480</dur><n>70</n><y>い</y><p>i</p><nStyle><v id="vibLen">24</v></nStyle></note></vsPart></vsTrack></vsq4>`)
 next:=72
 updated,err:=EditExisting(old,[]NoteEdit{{Tick:0,ExpectedPitch:69,NewPitch:&next}})
 if err!=nil{t.Fatal(err)}
 root,err:=parse(updated);if err!=nil{t.Fatal(err)}
 part:=root.all("vsTrack")[0].all("vsPart")[0]
 if len(part.all("mCtrl"))!=1||part.all("mCtrl")[0].child("v").Text!="4500"{t.Fatal("pitch bend automation lost")}
 ns:=part.all("note")
 if ns[0].text("n")!="72"||ns[0].child("nStyle").child("v").Text!="65"{t.Fatal("edited note expression changed")}
 if ns[1].text("n")!="70"||ns[1].child("nStyle").child("v").Text!="24"{t.Fatal("unmodified note expression changed")}
 if _,err:=EditExisting(old,[]NoteEdit{{Tick:0,ExpectedPitch:68,NewPitch:&next}});err==nil{t.Fatal("stale pitch edit accepted")}
}
