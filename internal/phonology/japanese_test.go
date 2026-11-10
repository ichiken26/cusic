package phonology

import (
 "testing"
)

func TestJapaneseHighVowelDevoicingContext(t *testing.T){
 cases:=[]struct{lyric string;next string;want string}{
  {"す","き","likely"}, // s-u before voiceless k
  {"す","ぐ",""},      // voiced g suppresses a categorical devoicing rule
  {"く","ゆ",""},      // くゆ: /j/ is voiced; do not drop this u
  {"し","た","likely"},
  {"つ","き","likely"},
 }
 for _,tt:=range cases{
  notes:=[]Note{{Tick:0,Duration:120,Lyric:tt.lyric},{Tick:120,Duration:240,Lyric:tt.next}}
  got:=Analyze(notes,184,4)
  if len(got.Findings)==0||len(got.Findings[0].Morae)==0{t.Fatalf("no analysis for %s",tt.lyric)}
  actual:=got.Findings[0].Morae[0].Devoicing
  if actual!=tt.want{t.Errorf("%s before %s: got %q want %q",tt.lyric,tt.next,actual,tt.want)}
 }
}
func TestKuyurasuDoesNotMoveNoteAcrossBar(t *testing.T){
 notes:=[]Note{{Tick:1920,Duration:480,Lyric:"くゆらす"}}
 result:=Analyze(notes,184,4)
 if len(result.Findings)!=1{t.Fatal("one note expected")}
 finding:=result.Findings[0]
 if len(finding.Morae)!=4{t.Fatalf("expected 4 morae for くゆらす, got %+v",finding.Morae)}
 if finding.Morae[0].Devoicing!=""{t.Fatal("く before ゆ was incorrectly devoiced")}
 if finding.Morae[3].Devoicing==""{t.Fatal("final す was not considered for devoicing")}
 if finding.Morae[0].StartTick!=1920||finding.Morae[3].EndTick!=2400{t.Fatal("morae exceeded the original note boundaries")}
 for i,m:=range finding.Morae{
  if m.StartTick<1920||m.EndTick>2400||m.EndTick<m.StartTick{t.Fatalf("mora %d slipped over bar: %+v",i,m)}
  if i>0&&m.StartTick!=finding.Morae[i-1].EndTick{t.Fatal("morae have a discontinuity")}
 }
}
func TestBarlinePreutteranceIsAdviceNotAutomaticShift(t *testing.T){
 result:=Analyze([]Note{{Tick:1920,Duration:240,Lyric:"す"}},184,4)
 f:=result.Findings[0]
 if !f.PreutteranceRisk||f.EstimatedLeadMs==0{t.Fatalf("missed preutterance risk: %+v",f)}
 if f.Morae[0].StartTick!=1920{t.Fatal("consonant was destructively advanced before barline")}
}
func TestKanaContractionsAndUnresolvedKanji(t *testing.T){
 ms,ok:=SplitMora("きゃく")
 if !ok||len(ms)!=2||ms[0].Text!="きゃ"{t.Fatalf("contracted mora split incorrectly: %+v",ms)}
 report:=Analyze([]Note{{Tick:0,Duration:480,Lyric:"揺らす"}},120,4)
 if len(report.Findings[0].Suggestions)==0{t.Fatal("unknown kanji falsely assigned phonemes")}
}
