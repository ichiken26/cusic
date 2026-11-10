// Package phonology gives conservative Japanese singing-pronunciation hints.
// A mora is not a fixed-duration acoustic syllable. Rules here are explicit
// heuristics, not transcribed audio or a trained forced aligner.
package phonology

import (
 "math"
 "sort"
 "strings"
 "unicode"
)

const PPQ int64 = 480

type Note struct {
 Tick int64 `json:"tick"`
 Duration int64 `json:"duration"`
 Lyric string `json:"lyric"`
 Pitch int `json:"pitch,omitempty"`
}
type Mora struct{
 Text string `json:"text"`
 Onset string `json:"onset,omitempty"`
 Vowel string `json:"vowel,omitempty"`
 Devoicing string `json:"devoicing,omitempty"`
 Reason string `json:"reason,omitempty"`
 StartTick int64 `json:"startTick"`
 EndTick int64 `json:"endTick"`
}
type Finding struct{
 Lyric string `json:"lyric"`
 Tick int64 `json:"tick"`
 Duration int64 `json:"duration"`
 Morae []Mora `json:"morae"`
 PreutteranceRisk bool `json:"preutteranceRisk"`
 EstimatedLeadMs int `json:"estimatedLeadMs,omitempty"`
 Crowded bool `json:"crowded"`
 Suggestions []string `json:"suggestions,omitempty"`
}
type Report struct {
 Findings []Finding `json:"findings"`
 Warnings []string `json:"warnings"`
 Policy string `json:"policy"`
}
type syllable struct{ onset,vowel string }
var plain=map[rune]syllable{
 'あ':{"","a"},'い':{"","i"},'う':{"","u"},'え':{"","e"},'お':{"","o"},
 'か':{"k","a"},'き':{"k","i"},'く':{"k","u"},'け':{"k","e"},'こ':{"k","o"},
 'さ':{"s","a"},'し':{"sh","i"},'す':{"s","u"},'せ':{"s","e"},'そ':{"s","o"},
 'た':{"t","a"},'ち':{"ch","i"},'つ':{"ts","u"},'て':{"t","e"},'と':{"t","o"},
 'な':{"n","a"},'に':{"n","i"},'ぬ':{"n","u"},'ね':{"n","e"},'の':{"n","o"},
 'は':{"h","a"},'ひ':{"h","i"},'ふ':{"f","u"},'へ':{"h","e"},'ほ':{"h","o"},
 'ま':{"m","a"},'み':{"m","i"},'む':{"m","u"},'め':{"m","e"},'も':{"m","o"},
 'や':{"y","a"},'ゆ':{"y","u"},'よ':{"y","o"},
 'ら':{"r","a"},'り':{"r","i"},'る':{"r","u"},'れ':{"r","e"},'ろ':{"r","o"},
 'わ':{"w","a"},'を':{"","o"},'ん':{"N",""},
 'が':{"g","a"},'ぎ':{"g","i"},'ぐ':{"g","u"},'げ':{"g","e"},'ご':{"g","o"},
 'ざ':{"z","a"},'じ':{"j","i"},'ず':{"z","u"},'ぜ':{"z","e"},'ぞ':{"z","o"},
 'だ':{"d","a"},'ぢ':{"j","i"},'づ':{"z","u"},'で':{"d","e"},'ど':{"d","o"},
 'ば':{"b","a"},'び':{"b","i"},'ぶ':{"b","u"},'べ':{"b","e"},'ぼ':{"b","o"},
 'ぱ':{"p","a"},'ぴ':{"p","i"},'ぷ':{"p","u"},'ぺ':{"p","e"},'ぽ':{"p","o"},
 'ゔ':{"v","u"},
}
func voicedOnset(o string)bool{
 switch o{case "k","s","sh","t","ch","ts","h","f","p":return false}
 return true
}
func isVoiceless(o string)bool{return o!=""&&!voicedOnset(o)}
func toHiragana(s string)string{
 return strings.Map(func(r rune)rune{
  if r>='ァ'&&r<='ヶ'{return r-0x60}
  return r
 },s)
}
func isSmallY(r rune)bool{return strings.ContainsRune("ゃゅょぁぃぅぇぉゎ",r)}
// SplitMora preserves contracted sounds (きゃ etc) in a single Japanese mora.
// Unknown kanji is reported as unsupported, never assigned fake durations.
func SplitMora(s string)([]Mora,bool){
 s=toHiragana(strings.TrimSpace(s))
 if s==""{return nil,false}
 out:=[]Mora{}
 for _,r:=range s{
  if r=='。'||r=='、'||unicode.IsSpace(r){continue}
  if isSmallY(r){
   if len(out)==0{return nil,false}
   old:=&out[len(out)-1]
   if old.Onset==""||old.Vowel!="i"{return nil,false}
   switch r{
   case 'ゃ','ぁ':old.Vowel="a"
   case 'ゅ','ぅ':old.Vowel="u"
   case 'ょ','ぉ':old.Vowel="o"
   case 'ぇ':old.Vowel="e"
   case 'ぃ':old.Vowel="i"
   default:return nil,false
   }
   old.Text+=string(r)
   old.Onset+="y"
   continue
  }
  if r=='っ' {out=append(out,Mora{Text:"っ",Onset:"cl"});continue}
  if r=='ー'||r=='-' {out=append(out,Mora{Text:string(r),Onset:"-",Vowel:""});continue}
  syll,ok:=plain[r];if !ok{return nil,false}
  out=append(out,Mora{Text:string(r),Onset:syll.onset,Vowel:syll.vowel})
 }
 if len(out)==0{return nil,false}
 return out,true
}
func moraWeight(m Mora)float64{
 if m.Text=="っ"{return .45}
 if m.Devoicing=="likely"{return .4}
 if m.Devoicing=="possible"{return .7}
 if m.Onset=="s"||m.Onset=="sh"||m.Onset=="ts"{return 1.25}
 return 1
}
func leadMS(onset string)int{
 switch onset{
 case "s","sh","f","h":return 45
 case "ts","ch":return 38
 case "k","t","p":return 25
 default:return 0
 }
}
func Analyze(input []Note,bpm float64,beatsPerBar int)Report{
 out:=Report{Findings:[]Finding{},Warnings:[]string{},
  Policy:"Acoustic timing estimates only; no original note onset/duration or SVP singer/retake data is changed. Japanese /i,u/ devoicing depends on consonant context, accent, tempo and singing style. Check against rendered WAV before committing."}
 if math.IsNaN(bpm)||math.IsInf(bpm,0)||bpm<=0{bpm=120;out.Warnings=append(out.Warnings,"BPM invalid; used 120 for approximate preutterance")}
 if beatsPerBar<=0{beatsPerBar=4;out.Warnings=append(out.Warnings,"Meter unavailable; assumed four quarter-note beats per bar")}
 notes:=append([]Note(nil),input...)
 sort.SliceStable(notes,func(i,j int)bool{return notes[i].Tick<notes[j].Tick})
 barTicks:=int64(beatsPerBar)*PPQ
 for i,n:=range notes{
  result:=Finding{Lyric:n.Lyric,Tick:n.Tick,Duration:n.Duration,Morae:[]Mora{},Suggestions:[]string{}}
  ms,ok:=SplitMora(n.Lyric)
  if !ok{
   result.Suggestions=append(result.Suggestions,"Reading could not be resolved to kana morae. Supply kana or a pronunciation dictionary before acoustic alignment.")
   out.Findings=append(out.Findings,result)
   continue
  }
  // Next onset may come from a following note; otherwise look inside the
  // current lyric. This avoids erroneously de-voicing く before ゆ.
  nextOnset:=""
  if i+1<len(notes) {
   nextMora,valid:=SplitMora(notes[i+1].Lyric)
   if valid&&len(nextMora)>0{nextOnset=nextMora[0].Onset}
  }
  pauseAfter:=i==len(notes)-1
  if !pauseAfter && notes[i+1].Tick>n.Tick+n.Duration+PPQ/8{pauseAfter=true}
  for j:=range ms{
   m:=&ms[j]
   if (m.Vowel=="i"||m.Vowel=="u")&&isVoiceless(m.Onset){
    following:=nextOnset
    atPause:=pauseAfter
    if j+1<len(ms){following=ms[j+1].Onset;atPause=false}
    if isVoiceless(following){
     m.Devoicing="likely"
     m.Reason="/i,u/ between voiceless consonants can be devoiced"
    }else if atPause {
     m.Devoicing="possible"
     m.Reason="phrase-final /i,u/ after voiceless consonant may be devoiced; sustained singing can retain voicing"
     if n.Duration<=PPQ/2{m.Devoicing="likely"}
    }
   }
  }
  if n.Duration<=0{result.Suggestions=append(result.Suggestions,"Invalid zero/negative note duration");result.Morae=ms;out.Findings=append(out.Findings,result);continue}
  if barTicks>0&&n.Tick%barTicks==0&&leadMS(ms[0].Onset)>0{
   result.PreutteranceRisk=true
   result.EstimatedLeadMs=leadMS(ms[0].Onset)
   result.Suggestions=append(result.Suggestions,"Consonant preutterance may sound before the barline. Keep the note at the beat; reduce phoneme lead-in/strength in SynthV rather than moving the entire note early.")
  }
  weights:=make([]float64,len(ms));sum:=0.0
  for j,m:=range ms{weights[j]=moraWeight(m);sum+=weights[j]}
  cursor:=n.Tick;cum:=0.0
  for j:=range ms{
   ms[j].StartTick=cursor
   cum+=weights[j]
   end:=n.Tick+int64(math.Round(float64(n.Duration)*cum/sum))
   if j==len(ms)-1{end=n.Tick+n.Duration}
   if end<cursor{end=cursor}
   ms[j].EndTick=end
   cursor=end
  }
  // A single short unvoiced phrase-final す may suppress its vowel.
  // For multi-mora lyrics use that mora's *allocated* span, not the entire
  // note duration (otherwise くゆらす incorrectly looks like a sustained す).
  for j:=range ms{
   if ms[j].Devoicing=="possible" && ms[j].EndTick-ms[j].StartTick<=PPQ/2 {
    ms[j].Devoicing="likely"
   }
  }
  if len(ms)>1 && float64(n.Duration)/float64(len(ms))<float64(PPQ)/4 {
   result.Crowded=true
   result.Suggestions=append(result.Suggestions,"Multiple morae are crowded into this note; split/move the phoneme boundaries WITHIN the existing note duration. Do not extend a vowel into the next bar.")
  }
  for _,m:=range ms{
   if m.Devoicing!=""{
    result.Suggestions=append(result.Suggestions,"Consider weakening the high vowel in "+m.Text+" ("+m.Devoicing+"); avoid forcing a full /i,u/ into the note. Keep pitched sustain if it is intentional.")
   }
  }
  result.Morae=ms
  out.Findings=append(out.Findings,result)
 }
 return out
}
