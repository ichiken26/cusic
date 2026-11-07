package store
import(
 "database/sql"
 "errors"
 "fmt"
 "sort"
)
// SyncPlan is a non-mutating comparison between an exported DAWproject and the SQLite project.
type SyncPlan struct {
 BaseRevision int64 `json:"baseRevision"`
 Tracks []SyncTrack `json:"tracks"`
}
type SyncTrack struct{
 Name string `json:"name"`
 Existing int `json:"existing"`
 Incoming int `json:"incoming"`
 Changed bool `json:"changed"`
 Skipped bool `json:"skipped"`
 Reason string `json:"reason,omitempty"`
}
func matchNotes(before []Note,after []InputNote)bool{
 if len(before)!=len(after){return false}
 a:=make([]InputNote,len(before));copy(a,after)
 x:=make([]InputNote,len(before))
 for i,n:=range before{x[i]=InputNote{Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Velocity:n.Velocity,Lyric:n.Lyric}}
 less:=func(ns []InputNote,i,j int)bool{
  if ns[i].Tick!=ns[j].Tick{return ns[i].Tick<ns[j].Tick}
  if ns[i].Pitch!=ns[j].Pitch{return ns[i].Pitch<ns[j].Pitch}
  return ns[i].Duration<ns[j].Duration
 }
 sort.Slice(x,func(i,j int)bool{return less(x,i,j)})
 sort.Slice(a,func(i,j int)bool{return less(a,i,j)})
 for i:=range x{if x[i]!=a[i]{return false}}
 return true
}
func (s *Store) PreviewSync(incoming []ImportedTrack)(SyncPlan,error){
 if len(incoming)==0{return SyncPlan{},errors.New("DAWproject has no imported note tracks")}
 revisions,err:=s.Revisions(1);if err!=nil{return SyncPlan{},err}
 plan:=SyncPlan{Tracks:[]SyncTrack{}};if len(revisions)>0{plan.BaseRevision=revisions[0].ID}
 seen:=map[string]bool{}
 for _,part:=range incoming{
  if seen[part.Name]{return plan,fmt.Errorf("ambiguous DAWproject track %q",part.Name)}
  seen[part.Name]=true
  t,err:=s.Track(part.Name);if err!=nil{return plan,fmt.Errorf("sync only supports existing track %q: %w",part.Name,err)}
  if t.Kind=="vocal" {plan.Tracks=append(plan.Tracks,SyncTrack{Name:t.Name,Skipped:true,Reason:"vocal notes/lyrics must be edited in SynthV or Piapro"});continue}
  for _,note:=range part.Notes{if err:=Validate(note);err!=nil{return plan,err}}
  previous,err:=s.Notes(t.ID,0,0);if err!=nil{return plan,err}
  plan.Tracks=append(plan.Tracks,SyncTrack{Name:t.Name,Existing:len(previous),Incoming:len(part.Notes),Changed:!matchNotes(previous,part.Notes)})
 }
 return plan,nil
}
func (s *Store) SyncTracks(incoming []ImportedTrack,expected int64)(int64,error){
 plan,err:=s.PreviewSync(incoming);if err!=nil{return 0,err}
 if plan.BaseRevision!=expected{return 0,fmt.Errorf("outdated sync: wanted HEAD %d, got %d",expected,plan.BaseRevision)}
 tx,err:=s.DB.Begin();if err!=nil{return 0,err};defer tx.Rollback()
 head,err:=latest(tx);if err!=nil{return 0,err}
 if head!=expected{return 0,fmt.Errorf("concurrent edit at revision %d; re-preview sync",head)}
 changes:=[]Change{}
 for i,part:=range incoming{
  if plan.Tracks[i].Skipped||!plan.Tracks[i].Changed{continue}
  var t Track
  if err:=tx.QueryRow("SELECT id,name,kind,singer FROM tracks WHERE name=?",part.Name).Scan(&t.ID,&t.Name,&t.Kind,&t.Singer);err!=nil{return 0,err}
  rows,err:=tx.Query("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE track_id=? ORDER BY tick,id",t.ID)
  if err!=nil{return 0,err}
  old:=[]Note{};for rows.Next(){var n Note;if err=rows.Scan(&n.ID,&n.TrackID,&n.Tick,&n.Duration,&n.Pitch,&n.Velocity,&n.Lyric);err!=nil{rows.Close();return 0,err};old=append(old,n)}
  err=rows.Err();rows.Close();if err!=nil{return 0,err}
  if _,err=tx.Exec("DELETE FROM notes WHERE track_id=?",t.ID);err!=nil{return 0,err}
  for _,n:=range old{changes=append(changes,newChange("note",n.ID,n,nil))}
  for _,src:=range part.Notes{
   n:=Note{ID:ID(),TrackID:t.ID,Tick:src.Tick,Duration:src.Duration,Pitch:src.Pitch,Velocity:src.Velocity,Lyric:src.Lyric}
   if err=putNote(tx,n);err!=nil{return 0,err}
   changes=append(changes,newChange("note",n.ID,nil,n))
  }
 }
 if len(changes)==0{return head,nil}
 rev,err:=record(tx,"dawproject.sync","Pull note edits from Cubase DAWproject",changes)
 if err!=nil{return 0,err};return rev,tx.Commit()
}
var _ = sql.ErrNoRows
