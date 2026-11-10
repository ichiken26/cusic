package svp

import (
 "errors"
 "encoding/json"
 "fmt"
 "os"

 "github.com/ichiken26/cusic/internal/phonology"
)

// AnalyzeTrack extracts lyric and timing information from an existing SVP
// without changing the project. It does not claim to have measured audio.
func AnalyzeTrack(data []byte,trackName string)(phonology.Report,error){
 var empty phonology.Report
 root,err:=decode(data);if err!=nil{return empty,err}
 tracks,err:=getTracks(root);if err!=nil{return empty,err}
 _,target,err:=findSource(tracks,trackName);if err!=nil{return empty,err}
 group,err:=getObject(target,"mainGroup");if err!=nil{return empty,err}
 raw,ok:=group["notes"].([]any);if !ok{return empty,errors.New("SVP track has no mainGroup.notes")}
 notes:=[]phonology.Note{}
 for i,item:=range raw{
  note,ok:=item.(map[string]any);if !ok{return empty,fmt.Errorf("invalid SVP note %d",i)}
  onset,err:=noteBlick(note,"onset");if err!=nil{return empty,err}
  duration,err:=noteBlick(note,"duration");if err!=nil{return empty,err}
  if duration<=0{return empty,fmt.Errorf("invalid SVP note duration at %d",i)}
  lyric,_:=note["lyrics"].(string)
  pitch:=0
  if p,ok:=note["pitch"].(json.Number);ok{v,e:=p.Int64();if e==nil{pitch=int(v)}}
  notes=append(notes,phonology.Note{Tick:onset*phonology.PPQ/BlickPerQuarter,Duration:duration*phonology.PPQ/BlickPerQuarter,Pitch:pitch,Lyric:lyric})
 }
 bpm,meter:=timing(root)
 return phonology.Analyze(notes,bpm,meter),nil
}
func AnalyzeFile(sourceFile,trackName string)(phonology.Report,error){
 data,err:=os.ReadFile(sourceFile);if err!=nil{return phonology.Report{},err}
 return AnalyzeTrack(data,trackName)
}
func timing(root map[string]any)(float64,int){
 bpm:=120.0
 meter:=4
 if timeline,ok:=root["time"].(map[string]any);ok{
  if tempos,ok:=timeline["tempo"].([]any);ok&&len(tempos)>0{
   if t,ok:=tempos[0].(map[string]any);ok{
    if v,ok:=t["bpm"].(json.Number);ok{if number,err:=v.Float64();err==nil{bpm=number}}
   }
  }
  if meters,ok:=timeline["meter"].([]any);ok&&len(meters)>0{
   if m,ok:=meters[0].(map[string]any);ok{
    if v,ok:=m["numerator"].(json.Number);ok{if number,err:=v.Int64();err==nil&&number>0{meter=int(number)}}
   }
  }
 }
 return bpm,meter
}
// MaxTick reports the end of the source track for replacing a complete
// cusic-managed vocal line without leaving old notes playing on the clone.
func MaxTick(data []byte,trackName string)(int64,error){
 root,err:=decode(data);if err!=nil{return 0,err}
 tracks,err:=getTracks(root);if err!=nil{return 0,err}
 _,target,err:=findSource(tracks,trackName);if err!=nil{return 0,err}
 group,err:=getObject(target,"mainGroup");if err!=nil{return 0,err}
 raw,ok:=group["notes"].([]any);if !ok{return 0,errors.New("source SVP has no mainGroup.notes")}
 var last int64
 for _,v:=range raw{
  item,ok:=v.(map[string]any);if !ok{return 0,errors.New("invalid source note")}
  onset,e:=noteBlick(item,"onset");if e!=nil{return 0,e}
  dur,e:=noteBlick(item,"duration");if e!=nil{return 0,e}
  if onset+dur>last{last=onset+dur}
 }
 return (last*phonology.PPQ+BlickPerQuarter-1)/BlickPerQuarter,nil
}
