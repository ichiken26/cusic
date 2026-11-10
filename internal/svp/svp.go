// Package svp applies cusic vocal payloads to a COPY of a SynthV project.
// Its original tracks, singer databases, tuning and parameter curves remain
// intact; the selected original track is muted, and a new cloned track carries
// the proposed replacement notes. The editor is never driven via a hidden UI.
package svp

import (
 "bytes"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "math"
 "os"
 "path/filepath"
 "sort"
 "strings"

 "github.com/ichiken26/cusic/internal/phonology"
)

const BlickPerQuarter int64 = 705600000
type Note struct{
 ID string `json:"id"`
 TrackID string `json:"trackId"`
 Tick int64 `json:"tick"`
 Duration int64 `json:"duration"`
 Pitch int `json:"pitch"`
 Velocity int `json:"velocity"`
 Lyric string `json:"lyric"`
}
type Payload struct{
 Version int `json:"version"`
 TrackID string `json:"trackId"`
 TrackName string `json:"trackName"`
 PPQ int64 `json:"ppq"`
 FromTick int64 `json:"fromTick"`
 ToTick int64 `json:"toTick"`
 Notes []Note `json:"notes"`
}
type Summary struct{
 SourceTrack string `json:"sourceTrack"`
 NewTrack string `json:"newTrack"`
 ReplacedNotes int `json:"replacedNotes"`
 AddedNotes int `json:"addedNotes"`
 MutedOriginal bool `json:"mutedOriginal"`
 Report phonology.Report `json:"pronunciation"`
 Warning string `json:"warning"`
}
func LoadPayload(data []byte)(Payload,error){
 var p Payload
 d:=json.NewDecoder(bytes.NewReader(data));d.DisallowUnknownFields()
 if err:=d.Decode(&p);err!=nil{return p,err}
 if p.Version!=1||p.PPQ<1||p.TrackID==""||p.TrackName==""||p.FromTick<0||p.ToTick<=p.FromTick||len(p.Notes)>100000 {
  return p,errors.New("invalid cusic SynthV payload header")
 }
 for i,n:=range p.Notes{
  if n.TrackID!=p.TrackID||n.Tick<p.FromTick||n.Duration<1||n.Tick>p.ToTick||
   n.Duration>p.ToTick-n.Tick||n.Pitch<0||n.Pitch>127||n.Velocity<1||n.Velocity>127{
   return p,fmt.Errorf("invalid SynthV payload note %d",i)
  }
 }
 return p,nil
}
func decode(data []byte)(map[string]any,error){
 if len(data)>64<<20{return nil,errors.New("SVP exceeds 64 MiB")}
 var root map[string]any
 decoder:=json.NewDecoder(bytes.NewReader(data));decoder.UseNumber()
 if err:=decoder.Decode(&root);err!=nil{return nil,fmt.Errorf("invalid SVP JSON: %w",err)}
 if root==nil{return nil,errors.New("SVP root must be an object")}
 if _,ok:=root["version"];!ok{return nil,errors.New("SVP version missing")}
 return root,nil
}
func getObject(obj map[string]any,key string)(map[string]any,error){
 v,ok:=obj[key].(map[string]any)
 if !ok{return nil,fmt.Errorf("SVP %q is not an object",key)}
 return v,nil
}
func getTracks(root map[string]any)([]any,error){
 tracks,ok:=root["tracks"].([]any)
 if !ok||len(tracks)==0{return nil,errors.New("SVP has no tracks")}
 return tracks,nil
}
func clone(source map[string]any)(map[string]any,error){
 b,err:=json.Marshal(source);if err!=nil{return nil,err}
 var out map[string]any
 decoder:=json.NewDecoder(bytes.NewReader(b));decoder.UseNumber()
 if err=decoder.Decode(&out);err!=nil{return nil,err}
 return out,nil
}
func randomUUID()(string,error){
 buf:=make([]byte,16)
 if _,err:=rand.Read(buf);err!=nil{return "",err}
 buf[6]=(buf[6]&0x0f)|0x40
 buf[8]=(buf[8]&0x3f)|0x80
 v:=hex.EncodeToString(buf)
 return v[:8]+"-"+v[8:12]+"-"+v[12:16]+"-"+v[16:20]+"-"+v[20:],nil
}
func blick(tick,ppq int64)(int64,error){
 if tick<0||ppq<=0{return 0,errors.New("invalid tick/PPQ")}
 if tick>math.MaxInt64/BlickPerQuarter{return 0,errors.New("note tick overflows SynthV blick")}
 return tick*BlickPerQuarter/ppq,nil
}
func noteBlick(raw map[string]any,field string)(int64,error){
 var value int64
 switch n:=raw[field].(type){
 case json.Number:
  parsed,err:=n.Int64();if err!=nil{return 0,fmt.Errorf("SVP note has invalid %s: %w",field,err)};value=parsed
 case int64:value=n
 case int:value=int64(n)
 default:return 0,fmt.Errorf("SVP note has invalid %s",field)
 }
 if value<0{return 0,fmt.Errorf("SVP note has negative %s",field)}
 return value,nil
}
func findSource(tracks []any,name string)(int,map[string]any,error){
 index:=-1
 var selected map[string]any
 for i,item:=range tracks{
  track,ok:=item.(map[string]any);if !ok{return -1,nil,fmt.Errorf("invalid SVP track %d",i)}
  label,_:=track["name"].(string)
  if label==name{
   if index!=-1{return -1,nil,fmt.Errorf("ambiguous SVP source track %q",name)}
   index=i;selected=track
  }
 }
 if index<0{return -1,nil,fmt.Errorf("source SVP track %q not found",name)}
 return index,selected,nil
}
func existingTrackName(tracks []any,name string)bool{
 for _,item:=range tracks{if t,ok:=item.(map[string]any);ok&&t["name"]==name{return true}}
 return false
}
func title(tracks []any,desired string)string{
 if strings.TrimSpace(desired)==""{desired="Teto"}
 stem:=desired+" (cusic)"
 if !existingTrackName(tracks,stem){return stem}
 for i:=2;;i++{
  alternative:=fmt.Sprintf("%s (%d)",stem,i)
  if !existingTrackName(tracks,alternative){return alternative}
 }
}
func validateSourceNoteRanges(raw []any,start,end int64)([]any,int,error){
 kept:=[]any{}
 replaced:=0
 for _,item:=range raw{
  note,ok:=item.(map[string]any);if !ok{return nil,0,errors.New("SVP mainGroup note malformed")}
  onset,err:=noteBlick(note,"onset");if err!=nil{return nil,0,err}
  dur,err:=noteBlick(note,"duration");if err!=nil||dur==0||onset>math.MaxInt64-dur{return nil,0,errors.New("invalid source note duration")}
  if onset<end&&onset+dur>start{
   if onset<start||onset+dur>end{return nil,0,errors.New("source note crosses payload boundary; widen the range instead of clipping tuned notes")}
   replaced++
  }else{kept=append(kept,item)}
 }
 return kept,replaced,nil
}
func ensureNewGroupIdentity(track map[string]any)error{
 group,err:=getObject(track,"mainGroup");if err!=nil{return err}
 newID,err:=randomUUID();if err!=nil{return err}
 oldID,_:=group["uuid"].(string)
 group["uuid"]=newID
 if reference,ok:=track["mainRef"].(map[string]any);ok{
  if oldID!="" {
   for _,key:=range []string{"groupID","groupId","uuid"}{
    if reference[key]==oldID{reference[key]=newID}
   }
  }
 }
 return nil
}
// Apply creates a new SVP JSON in memory. The supplied source bytes are not
// mutated or written. Tempo/meter and unrelated project tracks are preserved.
func Apply(source []byte,payload Payload,sourceTrack string)([]byte,Summary,error){
 result:=Summary{}
 if sourceTrack==""{return nil,result,errors.New("source track name required")}
 root,err:=decode(source);if err!=nil{return nil,result,err}
 tracks,err:=getTracks(root);if err!=nil{return nil,result,err}
 _,base,err:=findSource(tracks,sourceTrack);if err!=nil{return nil,result,err}
 mainGroup,err:=getObject(base,"mainGroup");if err!=nil{return nil,result,err}
 rawNotes,ok:=mainGroup["notes"].([]any);if !ok{return nil,result,errors.New("source track lacks mainGroup.notes")}
 if _,err=getObject(base,"mixer");err!=nil{return nil,result,err}
 if mainRef,ok:=base["mainRef"].(map[string]any);ok{
  if ref,ok:=mainRef["isInstrumental"].(bool);ok&&ref{return nil,result,errors.New("cannot clone an instrumental track as a vocal track")}
 }
 start,err:=blick(payload.FromTick,payload.PPQ);if err!=nil{return nil,result,err}
 end,err:=blick(payload.ToTick,payload.PPQ);if err!=nil{return nil,result,err}
 if start>=end{return nil,result,errors.New("empty blick range")}
 // Check all conditions before modifying the cloned project.
 kept,replaced,err:=validateSourceNoteRanges(rawNotes,start,end);if err!=nil{return nil,result,err}
 candidate,err:=clone(base);if err!=nil{return nil,result,err}
 if err=ensureNewGroupIdentity(candidate);err!=nil{return nil,result,err}
 newName:=title(tracks,payload.TrackName)
 candidate["name"]=newName
 if order,ok:=base["dispOrder"].(json.Number);ok{
  v,parseErr:=order.Int64();if parseErr==nil{candidate["dispOrder"]=v+1}
 }
 candidateMixer,err:=getObject(candidate,"mixer");if err!=nil{return nil,result,err}
 candidateMixer["mute"]=false
 candidateMixer["solo"]=false
 newGroup,err:=getObject(candidate,"mainGroup");if err!=nil{return nil,result,err}
 // Resolve the Japanese mora allocation before constructing notes.
 // A lyric such as くゆらす on one long note is split into contiguous
 // kana-mora note events, with a shorter share for high-vowel devoicing
 // candidates. All events remain WITHIN the original note range; do not
 // shift a consonant/note onset earlier than the barline.
 analysisNotes:=make([]phonology.Note,0,len(payload.Notes))
 for _,n:=range payload.Notes{
  analysisNotes=append(analysisNotes,phonology.Note{
   Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Lyric:n.Lyric,
  })
 }
 bpm,beatsPerBar:=timing(root)
 report:=phonology.Analyze(analysisNotes,bpm,beatsPerBar)
 replacement:=make([]any,0,len(payload.Notes))
 for i,n:=range payload.Notes{
  onset,err:=blick(n.Tick,payload.PPQ);if err!=nil{return nil,result,fmt.Errorf("note %d onset: %w",i,err)}
  duration,err:=blick(n.Duration,payload.PPQ);if err!=nil{return nil,result,fmt.Errorf("note %d duration: %w",i,err)}
  if duration==0{return nil,result,fmt.Errorf("note %d smaller than 1 blick",i)}
  // Never split short notes into zero-length events, or guess pronunciations
  // of kanji/foreign lyrics. Fall back to the literal lyric in those cases.
  morae:=report.Findings[i].Morae
  split:=len(morae)>1
  if split{
   for _,m:=range morae{if m.EndTick<=m.StartTick{split=false;break}}
  }
  if !split{
   replacement=append(replacement,map[string]any{"onset":onset,"duration":duration,
    "pitch":n.Pitch,"lyrics":n.Lyric,"phonemes":""})
   continue
  }
  for _,m:=range morae{
   startBlick,err:=blick(m.StartTick,payload.PPQ);if err!=nil{return nil,result,err}
   endBlick,err:=blick(m.EndTick,payload.PPQ);if err!=nil{return nil,result,err}
   if endBlick<=startBlick{return nil,result,errors.New("invalid zero-length mora subdivision")}
   replacement=append(replacement,map[string]any{"onset":startBlick,"duration":endBlick-startBlick,
    "pitch":n.Pitch,"lyrics":m.Text,"phonemes":""})
  }
 }
 merged:=append(append([]any{},kept...),replacement...)
 sort.SliceStable(merged,func(i,j int)bool{
  x,_:=merged[i].(map[string]any);y,_:=merged[j].(map[string]any)
  a,_:=noteBlick(x,"onset");b,_:=noteBlick(y,"onset");return a<b
 })
 newGroup["notes"]=merged
 // Source is muted ONLY in the generated output. Preserve all original
 // note objects, retakes and automation. New track keeps voice database and
 // group-level parameter curves from the cloned original (review after edit).
 originalMixer,_:=getObject(base,"mixer")
 originalMixer["mute"]=true
 originalMixer["solo"]=false
 // If an edited cusic track from a previous run exists, mute it as well in
 // the new copy, preventing accidental doubling when applying repeatedly.
 predecessor:=payload.TrackName+" (cusic)"
 for _,item:=range tracks{
  t,ok:=item.(map[string]any);if !ok||t==base{continue}
  name,_:=t["name"].(string)
  if name==predecessor||strings.HasPrefix(name,predecessor+" ("){
   if mx,ok:=t["mixer"].(map[string]any);ok{mx["mute"]=true;mx["solo"]=false}
  }
 }
 root["tracks"]=append(tracks,candidate)
 result=Summary{SourceTrack:sourceTrack,NewTrack:newName,MutedOriginal:true,ReplacedNotes:replaced,AddedNotes:len(replacement),Report:report,
  Warning:"Original source track is preserved/muted in new file. Multi-mora kana lyrics in new notes are divided within their old note boundaries by phonological timing weights; no original note crosses a barline. Devoicing is a contextual candidate, not a forced deletion of the vowel. Check the cloned track's pronunciation/retakes and pitch curves in SynthV."}
 output,err:=json.MarshalIndent(root,"","  ")
 if err!=nil{return nil,Summary{},err}
 return append(output,'\n'),result,nil
}
func ApplyFile(sourceFile,payloadFile,destination,sourceTrack string)(Summary,error){
 result:=Summary{}
 if sourceFile==""||payloadFile==""||destination==""{return result,errors.New("--from --payload --out are required")}
 if strings.ToLower(filepath.Ext(sourceFile))!=".svp"||strings.ToLower(filepath.Ext(destination))!=".svp"{
  return result,errors.New("SVP source/output must have .svp extension")
 }
 original,err:=os.ReadFile(sourceFile);if err!=nil{return result,err}
 data,err:=os.ReadFile(payloadFile);if err!=nil{return result,err}
 p,err:=LoadPayload(data);if err!=nil{return result,err}
 output,summary,err:=Apply(original,p,sourceTrack);if err!=nil{return result,err}
 if err=os.MkdirAll(filepath.Dir(destination),0755);err!=nil{return result,err}
 f,err:=os.OpenFile(destination,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0644)
 if err!=nil{return result,err}
 _,writeErr:=f.Write(output);closeErr:=f.Close()
 if writeErr!=nil||closeErr!=nil{os.Remove(destination);if writeErr!=nil{return result,writeErr};return result,closeErr}
 return summary,nil
}
