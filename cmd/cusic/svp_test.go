package main

import(
 "encoding/json"
 "os"
 "path/filepath"
 "testing"

 "github.com/ichiken26/cusic/internal/store"
)

const sourceSVP=`{
 "version":153,
 "time":{"tempo":[{"position":0,"bpm":184}],"meter":[{"index":0,"numerator":4,"denominator":4}]},
 "tracks":[{"name":"Teto","mixer":{"mute":false,"solo":false},
   "mainGroup":{"uuid":"teto-original-uuid","notes":[{"onset":0,"duration":705600000,"pitch":60,"lyrics":"す","attributes":{"custom":"KEEP"}}],
    "parameters":{"pitchDelta":{"mode":"cubic","points":[0,3]}}},
   "mainRef":{"groupID":"teto-original-uuid","database":{"name":"Kasane Teto SV2"}},"groups":[]}]
}`

func TestCLIApplyDirectlyFromCusicNotesToFreshSVP(t *testing.T){
 cwd,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 root:=t.TempDir()
 if err=os.Chdir(root);err!=nil{t.Fatal(err)}
 defer os.Chdir(cwd)
 if err=run([]string{"init","--name","Teto Demo","--tempo","184"});err!=nil{t.Fatal(err)}
 if err=run([]string{"track","add","--name","Teto","--kind","vocal","--singer","teto_sv2"});err!=nil{t.Fatal(err)}
 if err=run([]string{"note","add","--track","Teto","--tick","0","--duration","480","--pitch","62","--lyric","くゆらす"});err!=nil{t.Fatal(err)}
 if err=os.WriteFile("old.svp",[]byte(sourceSVP),0644);err!=nil{t.Fatal(err)}
 if err=run([]string{"vocal","analyze","--track","Teto"});err!=nil{t.Fatal(err)}
 if err=run([]string{"vocal","analyze","--track","Teto","--from","old.svp"});err!=nil{t.Fatal(err)}
 if err=run([]string{"svp","plan","--from","old.svp","--track","Teto"});err!=nil{t.Fatal(err)}
 if err=run([]string{"svp","apply","--from","old.svp","--track","Teto","--out","new.svp"});err!=nil{t.Fatal(err)}
 if err=run([]string{"svp","apply","--from","old.svp","--track","Teto","--out","new.svp"});err==nil{t.Fatal("same output overwritten")}
 original,err:=os.ReadFile("old.svp");if err!=nil{t.Fatal(err)}
 if string(original)!=sourceSVP{t.Fatal("original SVP was modified")}
 data,err:=os.ReadFile(filepath.Join(root,"new.svp"));if err!=nil{t.Fatal(err)}
 var project struct{
  Tracks []struct{
   Name string `json:"name"`
   Mixer struct{Mute bool `json:"mute"`} `json:"mixer"`
   MainGroup struct{Notes []struct{Lyric string `json:"lyrics"`} `json:"notes"`} `json:"mainGroup"`
  } `json:"tracks"`
 }
 if err=json.Unmarshal(data,&project);err!=nil{t.Fatal(err)}
 if len(project.Tracks)!=2||!project.Tracks[0].Mixer.Mute||project.Tracks[1].Mixer.Mute{
  t.Fatalf("original and clone mute status wrong: %+v",project.Tracks)
 }
 if len(project.Tracks[1].MainGroup.Notes)!=4{t.Fatalf("expected 4 Japanese mora notes in clone, got %+v",project.Tracks[1].MainGroup.Notes)}
 // Verify the original track still has its original lyrics.
 if project.Tracks[0].MainGroup.Notes[0].Lyric!="す"{t.Fatal("original retuned/deleted")}
 s,err:=store.Open(root);if err!=nil{t.Fatal(err)}
 if _,err=s.Track("Teto");err!=nil{t.Fatal(err)}
 s.Close()
}
