package vsqx

import (
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

// VoiceTemplate comes from a genuine VSQX voice table. cusic never
// generates or guesses a proprietary VOCALOID voicebank identifier.
type VoiceTemplate struct {
 File string `json:"file"`
 Voice string `json:"voice"`
 BankID string `json:"bankId,omitempty"`
 BS string `json:"bs,omitempty"`
 PC string `json:"pc,omitempty"`
}
type TemplateChoice struct {
 File string `json:"file"`
 Voice string `json:"voice"`
 Requested string `json:"requested,omitempty"`
 Fallback bool `json:"fallback"`
 Warning string `json:"warning,omitempty"`
}
func voiceRows(data []byte)([]VoiceTemplate,error){
 root,err:=parse(data);if err!=nil{return nil,err}
 if root.Name.Local!="vsq4" {return nil,errors.New("VOCALOID4 VSQX required")}
 table:=root.child("vVoiceTable")
 if table==nil{return nil,errors.New("VSQX lacks vVoiceTable")}
 result:=[]VoiceTemplate{}
 for _,v:=range table.all("vVoice"){
  voice:=v.text("vVoiceName")
  if voice==""{voice=v.text("name")}
  if voice==""{voice=v.text("id")}
  if voice==""{voice="(unnamed voice)"}
  result=append(result,VoiceTemplate{Voice:voice,BankID:v.text("compID"),BS:v.text("vBS"),PC:v.text("vPC")})
 }
 return result,nil
}

// ScanVoices only inspects this project's existing VSQX files. It never
// scans unrelated personal folders or reads vendor license/registry entries.
func ScanVoices(dir string)([]VoiceTemplate,error){
 root,err:=filepath.Abs(dir);if err!=nil{return nil,err}
 result:=[]VoiceTemplate{}
 visited:=0
 err=filepath.WalkDir(root,func(file string,e os.DirEntry,walkErr error)error{
  if walkErr!=nil{return walkErr}
  rel,err:=filepath.Rel(root,file);if err!=nil{return err}
  depth:=strings.Count(rel,string(os.PathSeparator))
  if e.IsDir(){
   if file!=root && (depth>=3||e.Name()==".git"||e.Name()==".cusic"||e.Name()=="node_modules"||e.Name()==".cusic-tools"){return filepath.SkipDir}
   return nil
  }
  if !strings.EqualFold(filepath.Ext(file),".vsqx"){return nil}
  visited++
  if visited>128{return errors.New("over 128 VSQX files in project; specify --template explicitly")}
  info,err:=e.Info();if err!=nil{return err}
  if info.Size()>24<<20{return nil} // never read arbitrary large files
  data,err:=os.ReadFile(file);if err!=nil{return err}
  voices,err:=voiceRows(data);if err!=nil{return nil}
  for _,v:=range voices{v.File=file;result=append(result,v)}
  return nil
 })
 if err!=nil{return nil,err}
 sort.Slice(result,func(i,j int)bool{
  if result[i].File!=result[j].File{return result[i].File<result[j].File}
  return result[i].Voice<result[j].Voice
 })
 return result,nil
}
func voiceMatch(request,available string)bool{
 r:=strings.ToLower(strings.TrimSpace(request))
 a:=strings.ToLower(strings.TrimSpace(available))
 if r==""||a==""{return false}
 if r==a{return true}
 // "Miku V4X Original" may appear as "MIKU_V4X_Original".
 if strings.Contains(r,"original") {return strings.Contains(a,"original")}
 for _,name:=range []string{"solid","sweet","soft","dark","vivid","light"}{
  if strings.Contains(r,name){return strings.Contains(a,name)}
 }
 return strings.Contains(a,r)
}
func ChooseTemplate(dir, requested, explicit string, allowFallback bool)(TemplateChoice,error){
 var out TemplateChoice
 if explicit!=""{
  abs,err:=filepath.Abs(explicit);if err!=nil{return out,err}
  data,err:=os.ReadFile(abs);if err!=nil{return out,err}
  voices,err:=voiceRows(data);if err!=nil{return out,err}
  if len(voices)==0{return out,errors.New("explicit VSQX template has no voices")}
  chosen:=voices[0]
  matched:=false
  for _,v:=range voices{if voiceMatch(requested,v.Voice){chosen=v;matched=true;break}}
  if requested!="" && !matched && !allowFallback{return out,fmt.Errorf("requested voice %q not found in template %s; add a VSQX containing that voice or pass allowFallback",requested,explicit)}
  out=TemplateChoice{File:abs,Voice:chosen.Voice,Requested:requested,Fallback:requested!=""&&!matched}
 }else{
  rows,err:=ScanVoices(dir);if err!=nil{return out,err}
  if len(rows)==0{return out,errors.New("no VOCALOID4 VSQX in project: export ONE VSQX from Piapro Studio with an installed voicebank; cusic will reuse it automatically afterwards")}
  chosen:=rows[0]
  matched:=false
  for _,v:=range rows{if voiceMatch(requested,v.Voice){chosen=v;matched=true;break}}
  if requested!=""&&!matched&&!allowFallback{
   names:=[]string{}
   for _,v:=range rows{names=append(names,v.Voice)}
   return out,fmt.Errorf("voice %q not found in project VSQX; available: %s; re-export with that voice or use allowFallback to reuse an available voice",requested,strings.Join(names,", "))
  }
  out=TemplateChoice{File:chosen.File,Voice:chosen.Voice,Requested:requested,Fallback:requested!=""&&!matched}
 }
 if out.Fallback{
  out.Warning=fmt.Sprintf("Requested voice %q is unavailable in project VSQX; exported with real voice %q instead. Select Original in Piapro after import if installed.",requested,out.Voice)
 }
 return out,nil
}

// SelectVoice switches the in-file singer reference to an existing voice table
// entry. Do not synthesize vBS/vPC or compID values that don't exist.
func SelectVoice(data []byte,requested string,allowFallback bool)([]byte,VoiceTemplate,error){
 root,err:=parse(data);if err!=nil{return nil,VoiceTemplate{},err}
 if root.Name.Local!="vsq4"{return nil,VoiceTemplate{},errors.New("VOCALOID4 VSQX required")}
 table:=root.child("vVoiceTable")
 if table==nil||len(table.all("vVoice"))==0{return nil,VoiceTemplate{},errors.New("VSQX has no actual voicebank metadata")}
 var selected *elem
 for _,v:=range table.all("vVoice"){
  if voiceMatch(requested,v.text("vVoiceName")){selected=v;break}
 }
 if selected==nil{
  if requested!=""&&!allowFallback{return nil,VoiceTemplate{},fmt.Errorf("voice %q not in template",requested)}
  selected=table.all("vVoice")[0]
 }
 selectedName:=selected.text("vVoiceName")
 if selectedName==""{selectedName=selected.text("id")}
 entry:=VoiceTemplate{Voice:selectedName,BankID:selected.text("compID"),BS:selected.text("vBS"),PC:selected.text("vPC")}
 if selected.text("vBS")!=""&&selected.text("vPC")!=""{
  for _,tr:=range root.all("vsTrack"){
   for _,part:=range tr.all("vsPart"){
    for _,singer:=range part.all("singer"){
     singer.set("vBS",selected.text("vBS"))
     singer.set("vPC",selected.text("vPC"))
    }
   }
  }
 }
 // Re-encoding changes formatting but preserves document elements.
 dataOut,err:=encodeVSQX(root)
 return dataOut,entry,err
}
