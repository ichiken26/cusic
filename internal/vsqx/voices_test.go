package vsqx

import (
 "bytes"
 "os"
 "path/filepath"
 "strings"
 "testing"

 "github.com/ichiken26/cusic/internal/store"
)

const voiceFixture = `<?xml version="1.0"?><vsq4 xmlns="http://www.yamaha.co.jp/vocaloid/schema/vsq4/">
<vVoiceTable>
<vVoice><vBS>0</vBS><vPC>1</vPC><compID>REAL_SOLID_ID</compID><vVoiceName>MIKU_V4X_Solid</vVoiceName></vVoice>
<vVoice><vBS>0</vBS><vPC>2</vPC><compID>REAL_ORIGINAL_ID</compID><vVoiceName>MIKU_V4X_Original</vVoiceName></vVoice>
</vVoiceTable><masterTrack><preMeasure>4</preMeasure><tempo><t>0</t><v>12000</v></tempo><timeSig><m>0</m><nu>4</nu><de>4</de></timeSig></masterTrack>
<vsTrack><name>Miku</name><vsPart><t>7680</t><playTime>1920</playTime><singer><t>0</t><vBS>0</vBS><vPC>1</vPC></singer>
<note><t>0</t><dur>480</dur><n>69</n><v>100</v><y>あ</y><p>a</p></note></vsPart></vsTrack></vsq4>`

func TestAutoVoiceSelectionAndSpecialLyrics(t *testing.T){
 dir:=t.TempDir()
 if err:=os.WriteFile(filepath.Join(dir,"template.vsqx"),[]byte(voiceFixture),0644);err!=nil{t.Fatal(err)}
 voices,err:=ScanVoices(dir);if err!=nil{t.Fatal(err)}
 if len(voices)!=2{t.Fatalf("expected 2 voice references, got %+v",voices)}
 chosen,err:=ChooseTemplate(dir,"Original","",false);if err!=nil{t.Fatal(err)}
 if chosen.Fallback||chosen.Voice!="MIKU_V4X_Original"{t.Fatalf("wrong voice: %+v",chosen)}
 track:=store.Track{ID:"v",Name:"SideVocal",Kind:"vocal"}
 notes:=[]store.Note{
  {ID:"n1",TrackID:"v",Tick:0,Duration:240,Pitch:60,Velocity:90,Lyric:"っ"},
  {ID:"n2",TrackID:"v",Tick:240,Duration:240,Pitch:62,Velocity:90,Lyric:"br1"},
  {ID:"n3",TrackID:"v",Tick:480,Duration:480,Pitch:64,Velocity:100,Lyric:"あ"},
 }
 b,selected,err:=ExportForVoice([]byte(voiceFixture),"Original",false,store.Settings{Name:"Demo",Tempo:184,MeterNum:4,MeterDen:4,PPQ:480},track,notes)
 if err!=nil{t.Fatal(err)}
 if selected.Voice!="MIKU_V4X_Original"{t.Fatalf("wrong exported voice %+v",selected)}
 root,err:=parse(b);if err!=nil{t.Fatal(err)}
 singer:=root.all("vsTrack")[0].all("vsPart")[0].child("singer")
 if singer.text("vPC")!="2"{t.Fatalf("wrong singer reference: %s",singer.text("vPC"))}
 if !bytes.Contains(b,[]byte("<p>Sil</p>"))||!bytes.Contains(b,[]byte("<p>br1</p>")){
  t.Fatalf("missing sokuon / breath lyrics in VSQX: %s",b)
 }
}
func TestMissingVoiceFallbackIsVisible(t *testing.T){
 dir:=t.TempDir()
 onlySolid:=strings.ReplaceAll(voiceFixture,
  "<vVoice><vBS>0</vBS><vPC>2</vPC><compID>REAL_ORIGINAL_ID</compID><vVoiceName>MIKU_V4X_Original</vVoiceName></vVoice>","")
 if err:=os.WriteFile(filepath.Join(dir,"solid.vsqx"),[]byte(onlySolid),0644);err!=nil{t.Fatal(err)}
 if _,err:=ChooseTemplate(dir,"Original","",false);err==nil{t.Fatal("strict mode ignored missing Original")}
 chosen,err:=ChooseTemplate(dir,"Original","",true);if err!=nil{t.Fatal(err)}
 if !chosen.Fallback||chosen.Warning==""||!strings.Contains(chosen.Voice,"Solid"){t.Fatalf("fallback not clearly reported: %+v",chosen)}
}
func TestExistingVsqxBreathEditPreservesAutomation(t *testing.T){
 source:=strings.Replace(voiceFixture,"<note><t>0</t>",`<mCtrl><t>20</t><v id="PIT">65</v></mCtrl><note><t>0</t>`,1)
 voice:="br3"
 updated,err:=EditExisting([]byte(source),[]NoteEdit{{Tick:0,ExpectedPitch:69,NewLyric:&voice}})
 if err!=nil{t.Fatal(err)}
 if !bytes.Contains(updated,[]byte("<p>br3</p>"))||!bytes.Contains(updated,[]byte("PIT")){t.Fatal("breath edit lost automation")}
}

func TestAutoExportFromComplexRealWorldVsqx(t *testing.T){
 complex:=strings.Replace(voiceFixture,
  "</masterTrack>", "<tempo><t>480</t><v>13200</v></tempo><timeSig><m>6</m><nu>3</nu><de>4</de></timeSig></masterTrack>",1)
 complex=strings.Replace(complex,"</vsPart>",`</vsPart><vsPart><t>9600</t><playTime>1920</playTime><note><t>0</t><dur>480</dur><n>70</n></note></vsPart>`,1)
 complex=strings.Replace(complex,"</vsTrack></vsq4>",`</vsTrack><vsTrack><name>Other</name><vsPart><t>7680</t><playTime>1920</playTime></vsPart></vsTrack></vsq4>`,1)
 track:=store.Track{ID:"new",Name:"SideVocal",Kind:"vocal"}
 notes:=[]store.Note{{ID:"1",TrackID:"new",Tick:0,Duration:480,Pitch:61,Velocity:100,Lyric:"あ"}}
 out,_,err:=ExportForVoice([]byte(complex),"Original",false,
  store.Settings{Name:"Song",Tempo:184,MeterNum:4,MeterDen:4,PPQ:480},track,notes)
 if err!=nil{t.Fatal(err)}
 root,err:=parse(out);if err!=nil{t.Fatal(err)}
 if len(root.all("vsTrack"))!=1{t.Fatal("unused vocal tracks were not removed from NEW export")}
 if len(root.all("vsTrack")[0].all("vsPart"))!=1{t.Fatal("unused parts remain")}
 master:=root.child("masterTrack")
 if len(master.all("tempo"))!=1||len(master.all("timeSig"))!=1{t.Fatal("template tempo/meter not normalized")}
 if master.child("tempo").text("v")!="18400"{t.Fatal("export wrong BPM")}
}
