package svp

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

const fixture=`{
 "version":153,
 "time":{"tempo":[{"position":0,"bpm":184}],"meter":[{"index":0,"numerator":4,"denominator":4}]},
 "renderConfig":{"sampleRate":44100,"bitDepth":24},
 "library":[{"name":"external-group-preserved"}],
 "tracks":[
  {"name":"Teto","dispOrder":0,"renderEnabled":true,"mixer":{"mute":false,"solo":false,"pan":0.2,"gainDecibel":-1},
   "mainGroup":{"uuid":"teto-group-source","name":"Original retakes","parameters":{"pitchDelta":{"mode":"cubic","points":[0,7]}},
    "notes":[{"onset":0,"duration":705600000,"pitch":60,"lyrics":"す","phonemes":"","attributes":{"manualRetake":"KEEP"}}]},
   "mainRef":{"groupID":"teto-group-source","database":{"name":"Kasane Teto SV2"},"isInstrumental":false},
   "groups":[]},
  {"name":"Other","mixer":{"mute":false,"solo":false},"mainGroup":{"notes":[]}}
 ]
}`
const edits=`{"version":1,"trackId":"cusic-teto","trackName":"Teto","ppq":480,
 "fromTick":0,"toTick":480,
 "notes":[{"id":"1","trackId":"cusic-teto","tick":0,"duration":480,"pitch":62,"velocity":100,"lyric":"くゆらす"}]}`

func TestApplyClonesNewTrackAndMutesOnlyOriginal(t *testing.T){
 p,err:=LoadPayload([]byte(edits));if err!=nil{t.Fatal(err)}
 data,result,err:=Apply([]byte(fixture),p,"Teto");if err!=nil{t.Fatal(err)}
 if !result.MutedOriginal||result.ReplacedNotes!=1||result.AddedNotes!=4{t.Fatalf("invalid summary %+v",result)}
 root,err:=decode(data);if err!=nil{t.Fatal(err)}
 tracks,err:=getTracks(root);if err!=nil{t.Fatal(err)}
 if len(tracks)!=3{t.Fatalf("expected 3 tracks, got %d",len(tracks))}
 original:=tracks[0].(map[string]any)
 replacement:=tracks[2].(map[string]any)
 originalMixer:=original["mixer"].(map[string]any)
 newMixer:=replacement["mixer"].(map[string]any)
 if originalMixer["mute"]!=true||newMixer["mute"]!=false{t.Fatal("original should be muted and edited clone audible")}
 if original["mainGroup"].(map[string]any)["uuid"]==replacement["mainGroup"].(map[string]any)["uuid"]{t.Fatal("duplicated source group ID")}
 if replacement["mainRef"].(map[string]any)["groupID"]!=replacement["mainGroup"].(map[string]any)["uuid"]{t.Fatal("new reference does not point to new group")}
 if replacement["mainRef"].(map[string]any)["database"].(map[string]any)["name"]!="Kasane Teto SV2"{t.Fatal("voicebank lost")}
 originalNotes:=original["mainGroup"].(map[string]any)["notes"].([]any)
 if originalNotes[0].(map[string]any)["lyrics"]!="す"{t.Fatal("original tuned note was deleted")}
 if originalNotes[0].(map[string]any)["attributes"].(map[string]any)["manualRetake"]!="KEEP"{t.Fatal("original vocal retake lost")}
 cloneNotes:=replacement["mainGroup"].(map[string]any)["notes"].([]any)
 if len(cloneNotes)!=4||cloneNotes[0].(map[string]any)["lyrics"]!="く"||cloneNotes[3].(map[string]any)["lyrics"]!="す"{t.Fatal("mora-aware edited notes not applied")}
 var previousEnd int64
 for i,item:=range cloneNotes{
  note:=item.(map[string]any)
  begin,err:=noteBlick(note,"onset");if err!=nil{t.Fatal(err)}
  dur,err:=noteBlick(note,"duration");if err!=nil{t.Fatal(err)}
  if i>0&&begin!=previousEnd{t.Fatal("mora timing discontinuity")}
  previousEnd=begin+dur
 }
 if previousEnd!=BlickPerQuarter{t.Fatal("mora notes exceeded initial note duration")}
 if len(result.Report.Findings)!=1||len(result.Report.Findings[0].Morae)!=4{t.Fatal("phonological report missing くゆらす")}
 if result.Report.Findings[0].Morae[0].Devoicing!=""{t.Fatal("wrongly devoiced く before ゆ")}
 if result.Report.Findings[0].Morae[3].Devoicing==""{t.Fatal("did not flag phrase-final す")}
 if !strings.Contains(string(data),"external-group-preserved"){t.Fatal("unrelated project data changed")}
 if !strings.Contains(string(data),"pitchDelta"){t.Fatal("original parameter curves lost")}
 // Never change the supplied file contents in memory.
 if !strings.Contains(fixture,`"mute":false`){t.Fatal("fixture corrupted")}
}
func TestSVPRejectCrossBoundary(t *testing.T){
 original:=strings.Replace(fixture,`"duration":705600000`,`"duration":1411200000`,1)
 p,e:=LoadPayload([]byte(edits));if e!=nil{t.Fatal(e)}
 if _,_,e=Apply([]byte(original),p,"Teto");e==nil{t.Fatal("crossing original note would be clipped")}
}
func TestSVPNonOverwrite(t *testing.T){
 root:=t.TempDir()
 source:=filepath.Join(root,"input.svp")
 payload:=filepath.Join(root,"notes.json")
 output:=filepath.Join(root,"new.svp")
 if err:=os.WriteFile(source,[]byte(fixture),0644);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(payload,[]byte(edits),0644);err!=nil{t.Fatal(err)}
 if _,err:=ApplyFile(source,payload,output,"Teto");err!=nil{t.Fatal(err)}
 if _,err:=ApplyFile(source,payload,output,"Teto");err==nil{t.Fatal("silently overwrote existing SVP")}
 original,err:=os.ReadFile(source);if err!=nil{t.Fatal(err)}
 if string(original)!=fixture{t.Fatal("original source was changed")}
 var parsed map[string]any
 b,err:=os.ReadFile(output);if err!=nil{t.Fatal(err)}
 if err=json.Unmarshal(b,&parsed);err!=nil{t.Fatal(err)}
}
func TestSVPAnalyzeReadOnly(t *testing.T){
 report,err:=AnalyzeTrack([]byte(fixture),"Teto");if err!=nil{t.Fatal(err)}
 if len(report.Findings)!=1||report.Findings[0].Lyric!="す"{t.Fatalf("analysis incorrect: %+v",report)}
 if _,err:=AnalyzeTrack([]byte(fixture),"Unknown");err==nil{t.Fatal("missing track not reported")}
}
