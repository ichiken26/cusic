package main

import(
 "os"
 "path/filepath"
 "strings"
 "testing"
)
func TestOneFlowVocalCreationAndAutoVSQX(t *testing.T){
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 root:=t.TempDir()
 if err=os.Chdir(root);err!=nil{t.Fatal(err)}
 defer os.Chdir(cwd)
 if err=run([]string{"init","--name","SideVocal Test","--tempo","184"});err!=nil{t.Fatal(err)}
 source:=`<?xml version="1.0"?><vsq4 xmlns="http://www.yamaha.co.jp/vocaloid/schema/vsq4/"><vVoiceTable>
 <vVoice><vBS>0</vBS><vPC>1</vPC><compID>TEST_ORIGINAL_BANK</compID><vVoiceName>MIKU_V4X_Original</vVoiceName></vVoice>
 </vVoiceTable><masterTrack><seqName>template</seqName><resolution>480</resolution><preMeasure>4</preMeasure><timeSig><m>0</m><nu>4</nu><de>4</de></timeSig><tempo><t>0</t><v>12000</v></tempo></masterTrack>
 <vsTrack><name>Miku</name><vsPart><t>7680</t><playTime>1920</playTime><singer><t>0</t><vBS>0</vBS><vPC>1</vPC></singer><note><t>0</t><dur>480</dur><n>60</n><v>100</v><y>あ</y><p>a</p></note></vsPart></vsTrack></vsq4>`
 if err=os.WriteFile("existing-piapro.vsqx",[]byte(source),0644);err!=nil{t.Fatal(err)}
 noteFile:=`{"track":"SideVocal","singer":"Original","createTrack":true,"baseRevision":0,"changes":[
 {"action":"add","note":{"tick":0,"duration":240,"pitch":66,"velocity":100,"lyric":"っ"}},
 {"action":"add","note":{"tick":240,"duration":240,"pitch":67,"velocity":100,"lyric":"br1"}},
 {"action":"add","note":{"tick":480,"duration":480,"pitch":69,"velocity":100,"lyric":"あ"}}
 ]}`
 if err=os.WriteFile("vocal-notes.json",[]byte(noteFile),0644);err!=nil{t.Fatal(err)}
 if err=run([]string{"vocal","voices"});err!=nil{t.Fatal(err)}
 if err=run([]string{"vocal","apply","--file","vocal-notes.json"});err!=nil{t.Fatal(err)}
 if err=run([]string{"export","--format","vsqx","--track","SideVocal","--out","rendered.vsqx"});err!=nil{t.Fatal(err)}
 b,err:=os.ReadFile(filepath.Join(root,"rendered.vsqx"));if err!=nil{t.Fatal(err)}
 for _,value:=range []string{"TEST_ORIGINAL_BANK","MIKU_V4X_Original","<p>Sil</p>","<p>br1</p>","<v>18400</v>"}{
  if !strings.Contains(string(b),value){t.Fatalf("missing %q in auto-exported VSQX: %s",value,b)}
 }
 if err=run([]string{"export","--format","vsqx","--track","SideVocal","--out","rendered.vsqx"});err==nil{t.Fatal("overwrote existing VSQX")}
}
