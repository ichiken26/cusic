package mix

import (
 "archive/zip"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strconv"
 "strings"

 "github.com/ichiken26/cusic/internal/store"
)

// ExportPreserving uses the original DAWproject as the source of truth for all
// plug-ins, instrument states, routing, automation, audio and arbitrary tracks.
// If no imported notes changed, the resulting archive is byte-identical.
// Edited notes can be patched only for uncomplicated single-clip note lanes.
func ExportPreserving(source,destination string,changed map[string][]store.Note)error{
 if source==destination{return errors.New("source and output must differ")}
 if _,err:=os.Stat(destination);err==nil{return errors.New("output already exists")}else if !errors.Is(err,os.ErrNotExist){return err}
 if len(changed)==0{
  input,err:=os.Open(source);if err!=nil{return err}
  defer input.Close()
  if err=os.MkdirAll(filepath.Dir(destination),0755);err!=nil{return err}
  temp,err:=os.CreateTemp(filepath.Dir(destination),".cusic-unchanged-*");if err!=nil{return err}
  defer os.Remove(temp.Name())
  if _,err=io.Copy(temp,input);err!=nil{temp.Close();return err}
  if err=temp.Close();err!=nil{return err}
  return os.Rename(temp.Name(),destination)
 }
 reader,project,err:=readDAWProject(source);if err!=nil{return err}
 defer reader.Close()
 for trackName,ns:=range changed{
  if err=patchTrackNotes(project,trackName,ns);err!=nil{return err}
 }
 data,err:=xmlBytes(project);if err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(destination),0755);err!=nil{return err}
 temp,err:=os.CreateTemp(filepath.Dir(destination),".cusic-preserved-*");if err!=nil{return err}
 defer os.Remove(temp.Name())
 writer:=zip.NewWriter(temp)
 seen:=map[string]bool{}
 for _,entry:=range reader.File{
  if !safeZipName(entry.Name)||seen[entry.Name]{writer.Close();temp.Close();return fmt.Errorf("unsafe/duplicate source entry %q",entry.Name)}
  seen[entry.Name]=true
  if entry.Name!="project.xml"{
   // ZIP compressed data is copied without decoding or re-encoding plug-in
   // state chunks and other media assets.
   if err=writer.Copy(entry);err!=nil{writer.Close();temp.Close();return err}
   continue
  }
  out,e:=writer.CreateHeader(&entry.FileHeader);if e!=nil{writer.Close();temp.Close();return e}
  if _,e=out.Write(data);e!=nil{writer.Close();temp.Close();return e}
 }
 if err=writer.Close();err!=nil{temp.Close();return err}
 if err=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),destination)
}

func patchTrackNotes(project *node,name string,notes []store.Note)error{
 track,err:=locateTrack(project,name);if err!=nil{return err}
 trackID:=track.attr("id")
 if trackID==""{return fmt.Errorf("track %q lacks DAWproject id",name)}
 arrangement:=project.child("Arrangement")
 if arrangement==nil{return errors.New("DAWproject has no Arrangement")}
 candidates:=[]*node{}
 var scan func(*node,string)
 scan=func(n *node,inherited string){
  if n.Name.Local=="Lanes" && n.attr("track")!=""{inherited=n.attr("track")}
  if n.Name.Local=="Lanes" && inherited==trackID {candidates=append(candidates,n)}
  for _,item:=range n.Children{if child,ok:=item.(*node);ok{scan(child,inherited)}}
 }
 scan(arrangement,"")
 // Need exactly one lane explicitly assigned to the target track. More than
 // one independent note region requires a dedicated arrangement editor.
 var lane *node
 for _,v:=range candidates{if v.attr("track")==trackID{
  if lane!=nil{return fmt.Errorf("track %q has multiple lanes; cannot patch without altering arrangement",name)}
  lane=v
 }}
 if lane==nil{return fmt.Errorf("track %q has no editable DAWproject note lane",name)}
 if lane.attr("timeUnit")=="seconds"{return fmt.Errorf("track %q uses seconds; note editing requires beat time",name)}
 var list *node
 var noteClip *node
 var gather func(*node)error
 gather=func(n *node)error{
  if n.Name.Local=="Lanes" && n!=lane && n.attr("track")!="" && n.attr("track")!=trackID{return nil}
  if n.Name.Local=="Notes"{
   if list!=nil{return fmt.Errorf("track %q has multiple note regions; refusing destructive flattening",name)}
   list=n
  }
  if n.Name.Local=="Clip"{
   for _,part:=range n.Children{if child,ok:=part.(*node);ok&&child.Name.Local=="Notes"{noteClip=n}}
  }
  for _,part:=range n.Children{if child,ok:=part.(*node);ok{
   if err:=gather(child);err!=nil{return err}
  }}
  }
  return nil
 }
 if err=gather(lane);err!=nil{return err}
 if list==nil{
  if len(notes)==0{return nil}
  // No existing note region: add a standard zero-offset clip but only when
  // the track lane is otherwise empty.
  for _,item:=range lane.Children{if child,ok:=item.(*node);ok&&(child.Name.Local=="Clips"||child.Name.Local=="Audio"||child.Name.Local=="Warps"){
   return fmt.Errorf("track %q has clips/audio; cannot inject notes safely",name)
  }}
  list=element("Notes");list.setAttr("id",freshID(project,"notes:"+name))
  noteClip=element("Clip")
  noteClip.setAttr("time","0");noteClip.setAttr("playStart","0")
  noteClip.Children=append(noteClip.Children,list)
  clips:=element("Clips");clips.setAttr("id",freshID(project,"clips:"+name))
  clips.Children=append(clips.Children,noteClip)
  lane.Children=append(lane.Children,clips)
 }
 if noteClip!=nil{
  if noteClip.attr("time")!=""&&noteClip.attr("time")!="0"{return fmt.Errorf("track %q note clip starts away from beat 0; refusing destructive rewrite",name)}
  if noteClip.attr("playStart")!=""&&noteClip.attr("playStart")!="0"{return fmt.Errorf("track %q note clip has playStart; refusing destructive rewrite",name)}
  for _,part:=range noteClip.Children{if child,ok:=part.(*node);ok&&child!=list{
   return fmt.Errorf("track %q note clip contains extra data (%s); refusing to discard it",name,child.Name.Local)
  }}
 }
 for _,item:=range list.Children{
  existing,ok:=item.(*node);if !ok{continue}
  if existing.Name.Local!="Note"{return fmt.Errorf("track %q has nested notes/automation; refusing to discard it",name)}
  if len(existing.Children)!=0{return fmt.Errorf("track %q has note expression; refusing to discard it",name)}
  for _,a:=range existing.Attr{
   switch a.Name.Local{case "time","duration","channel","key","vel","rel":
   default:return fmt.Errorf("track %q note has extra attribute %s; refusing destructive replacement",name,a.Name.Local)}
  }
 }
 if len(notes)>200000{return errors.New("too many notes in a DAWproject track")}
 list.Children=nil
 end:=float64(0)
 for _,item:=range notes{
  if err=store.Validate(store.InputNote{Tick:item.Tick,Duration:item.Duration,Pitch:item.Pitch,Velocity:item.Velocity});err!=nil{return err}
  n:=element("Note")
  n.setAttr("time",strconv.FormatFloat(float64(item.Tick)/float64(store.PPQ),'f',8,64))
  n.setAttr("duration",strconv.FormatFloat(float64(item.Duration)/float64(store.PPQ),'f',8,64))
  n.setAttr("channel","0")
  n.setAttr("key",strconv.Itoa(item.Pitch))
  n.setAttr("vel",strconv.FormatFloat(float64(item.Velocity)/127,'f',8,64))
  n.setAttr("rel",strconv.FormatFloat(float64(item.Velocity)/127,'f',8,64))
  list.Children=append(list.Children,n)
  candidate:=float64(item.Tick+item.Duration)/float64(store.PPQ)
  if candidate>end{end=candidate}
 }
 if noteClip!=nil{
  current,parseErr:=strconv.ParseFloat(noteClip.attr("duration"),64)
  if parseErr!=nil{current=0}
  if end>current{noteClip.setAttr("duration",strconv.FormatFloat(end,'f',8,64))}
 }
 return nil
}
