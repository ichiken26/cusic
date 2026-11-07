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
