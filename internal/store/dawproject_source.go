package store

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

const sourceIndexName="dawproject-source.json"

type SourceDAWProject struct{
 Archive string `json:"archive"`
 SHA256 string `json:"sha256"`
 ImportedNoteHashes map[string]string `json:"importedNoteHashes"`
}

// noteFingerprint uses musical values rather than internal randomly-assigned
// SQLite IDs, so unchanged notes do not get needlessly reserialized.
func (s *Store) noteFingerprint(trackID string)(string,error){
 ns,err:=s.Notes(trackID,0,0);if err!=nil{return "",err}
 type stable struct{Tick,Duration int64;Pitch,Velocity int}
 values:=make([]stable,0,len(ns))
 for _,n:=range ns{values=append(values,stable{n.Tick,n.Duration,n.Pitch,n.Velocity})}
 sort.Slice(values,func(i,j int)bool{
  a,b:=values[i],values[j]
  if a.Tick!=b.Tick{return a.Tick<b.Tick}
  if a.Pitch!=b.Pitch{return a.Pitch<b.Pitch}
  if a.Duration!=b.Duration{return a.Duration<b.Duration}
  return a.Velocity<b.Velocity
 })
 data,err:=json.Marshal(values);if err!=nil{return "",err}
 sum:=sha256.Sum256(data)
 return hex.EncodeToString(sum[:]),nil
}
func (s *Store) SourceDAWProject()(*SourceDAWProject,error){
 index:=filepath.Join(s.Root,".cusic",sourceIndexName)
 data,err:=os.ReadFile(index)
 if errors.Is(err,os.ErrNotExist){return nil,nil}
 if err!=nil{return nil,err}
 var snapshot SourceDAWProject
 if err=json.Unmarshal(data,&snapshot);err!=nil{return nil,err}
 if snapshot.Archive==""||strings.ContainsAny(snapshot.Archive,"/\\")||
  !strings.HasPrefix(snapshot.Archive,"source-")||!strings.HasSuffix(snapshot.Archive,".dawproject")||
  len(snapshot.SHA256)!=64{return nil,errors.New("corrupt DAWproject source index")}
 for _,c:=range snapshot.SHA256{if !strings.ContainsRune("0123456789abcdef",c){return nil,errors.New("invalid DAWproject SHA256")}}
 if snapshot.ImportedNoteHashes==nil{return nil,errors.New("missing imported note hashes")}
 return &snapshot,nil
}
func (s *Store) SourceArchive(src *SourceDAWProject)(string,error){
 if src==nil{return "",errors.New("no source DAWproject imported")}
 path:=filepath.Join(s.Root,".cusic",src.Archive)
 f,err:=os.Open(path);if err!=nil{return "",err}
 defer f.Close()
 h:=sha256.New()
 if _,err=io.Copy(h,f);err!=nil{return "",err}
 if hex.EncodeToString(h.Sum(nil))!=src.SHA256{return "",errors.New("saved source DAWproject hash changed: refusing export")}
 return path,nil
}
// SaveSourceDAWProject captures the original ZIP (including opaque VST states)
// only AFTER successful note/audio import. A different original is not silently
// substituted: export must never lose an established plug-in configuration.
func (s *Store) SaveSourceDAWProject(source string,importedNames []string)error{
 f,err:=os.Open(source);if err!=nil{return err}
 defer f.Close()
 info,err:=f.Stat();if err!=nil{return err}
 if info.Size()>4<<30{return errors.New("source DAWproject exceeds 4GiB")}
 hash:=sha256.New()
 if _,err=io.Copy(hash,f);err!=nil{return err}
 digest:=hex.EncodeToString(hash.Sum(nil))
 current,err:=s.SourceDAWProject();if err!=nil{return err}
 if current!=nil{
  if current.SHA256==digest{return nil}
  return errors.New("another DAWproject source is already registered; refusing silent replacement of effects and instrument states")
 }
 snapshot:=SourceDAWProject{
  Archive:"source-"+digest+".dawproject",SHA256:digest,ImportedNoteHashes:map[string]string{},
 }
 for _,name:=range importedNames{
  track,err:=s.Track(name);if err!=nil{return fmt.Errorf("imported note track %q: %w",name,err)}
  h,err:=s.noteFingerprint(track.ID);if err!=nil{return err}
  snapshot.ImportedNoteHashes[name]=h
 }
 directory:=filepath.Join(s.Root,".cusic")
 if err=os.MkdirAll(directory,0755);err!=nil{return err}
 archive:=filepath.Join(directory,snapshot.Archive)
 if _,err=os.Stat(archive);errors.Is(err,os.ErrNotExist){
  if _,err=f.Seek(0,io.SeekStart);err!=nil{return err}
  dst,err:=os.CreateTemp(directory,".source-*");if err!=nil{return err}
  defer os.Remove(dst.Name())
  h:=sha256.New()
  _,err=io.Copy(io.MultiWriter(dst,h),f)
  if err!=nil{dst.Close();return err}
  if err=dst.Close();err!=nil{return err}
  if hex.EncodeToString(h.Sum(nil))!=digest{return errors.New("DAWproject source changed during copy")}
  if err=os.Rename(dst.Name(),archive);err!=nil{return err}
 }else if err!=nil{return err}
 data,err:=json.MarshalIndent(snapshot,"","  ");if err!=nil{return err}
 index:=filepath.Join(directory,sourceIndexName)
 tmp,err:=os.CreateTemp(directory,".source-index-*");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if _,err=tmp.Write(data);err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 if _,err=os.Stat(index);err==nil{return errors.New("DAWproject source index already exists")}
 return os.Rename(tmp.Name(),index)
}
// ChangesSinceSource returns only note tracks that changed since import.
// New note tracks are refused rather than silently omitted from the export.
func (s *Store) ChangesSinceSource(src *SourceDAWProject)(map[string][]Note,error){
 if src==nil{return nil,errors.New("missing source DAWproject")}
 tracks,err:=s.Tracks();if err!=nil{return nil,err}
 result:=map[string][]Note{}
 for _,track:=range tracks{
  h,err:=s.noteFingerprint(track.ID);if err!=nil{return nil,err}
  original,exists:=src.ImportedNoteHashes[track.Name]
  if !exists{
   notes,err:=s.Notes(track.ID,0,0);if err!=nil{return nil,err}
   if len(notes)>0{return nil,fmt.Errorf("new note track %q cannot be mapped safely to original DAWproject; use a separate export, original VST states are protected",track.Name)}
   continue
  }
  if h!=original{
   notes,err:=s.Notes(track.ID,0,0);if err!=nil{return nil,err}
   result[track.Name]=notes
  }
 }
 for name:=range src.ImportedNoteHashes{if _,err:=s.Track(name);err!=nil{return nil,fmt.Errorf("original note track %q disappeared; cannot preserve DAWproject safely",name)}}
 return result,nil
}

// VerifySourceNotes is used when attaching an old DAWproject to a cusic
// project imported before source archiving existed. Never attach an unrelated
// source (which might have different plug-ins or note arrangements).
func (s *Store) VerifySourceNotes(imported []ImportedTrack) error {
 settings,err:=s.Settings();if err!=nil{return err}
 // Verify tempo/meter in the caller before attaching; imported tracks hold
 // note values only. Empty original notes are accepted if names match.
 if settings.Tempo==0{return errors.New("project BPM is not confirmed")}
 tracks,err:=s.Tracks();if err!=nil{return err}
 if len(imported)==0 && len(tracks)>0{
  return errors.New("no note tracks in original DAWproject; verify the source and import it afresh")
 }
 for _,tr:=range imported{
  current,err:=s.Track(tr.Name);if err!=nil{return fmt.Errorf("source note track %q is not in this project: %w",tr.Name,err)}
  currentNotes,err:=s.Notes(current.ID,0,0);if err!=nil{return err}
  type value struct {Tick,Duration int64;Pitch,Velocity int}
  have:=make([]value,0,len(currentNotes))
  want:=make([]value,0,len(tr.Notes))
  for _,n:=range currentNotes{have=append(have,value{n.Tick,n.Duration,n.Pitch,n.Velocity})}
  for _,n:=range tr.Notes{want=append(want,value{n.Tick,n.Duration,n.Pitch,n.Velocity})}
  sort.Slice(have,func(i,j int)bool{return valueLess(have[i],have[j])})
  sort.Slice(want,func(i,j int)bool{return valueLess(want[i],want[j])})
  if len(have)!=len(want){return fmt.Errorf("note mismatch on %q: original DAWproject cannot safely be attached",tr.Name)}
  for i:=range have{if have[i]!=want[i]{return fmt.Errorf("notes on %q differ from source DAWproject; refusing unsafe attach",tr.Name)}}
 }
 return nil
}
func valueLess(a,b struct{Tick,Duration int64;Pitch,Velocity int})bool{
 if a.Tick!=b.Tick{return a.Tick<b.Tick}
 if a.Pitch!=b.Pitch{return a.Pitch<b.Pitch}
 if a.Duration!=b.Duration{return a.Duration<b.Duration}
 return a.Velocity<b.Velocity
}
