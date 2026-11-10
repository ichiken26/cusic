package store

import (
 "database/sql"
 "errors"
 "fmt"
 "strings"
)

// VocalChange edits specific notes by ID instead of clearing an entire bar.
// New vocal phrases can be inserted without touching already tuned phrases.
type VocalChange struct {
 Action string `json:"action"` // add, update, delete
 NoteID string `json:"noteId,omitempty"`
 Note *InputNote `json:"note,omitempty"`
 Pitch *int `json:"pitch,omitempty"`
 Lyric *string `json:"lyric,omitempty"`
 Velocity *int `json:"velocity,omitempty"`
 Tick *int64 `json:"tick,omitempty"`
 Duration *int64 `json:"duration,omitempty"`
}
type VocalPatch struct {
 Track string `json:"track"`
 Singer string `json:"singer,omitempty"`
 BaseRevision int64 `json:"baseRevision"`
 Message string `json:"message,omitempty"`
 Changes []VocalChange `json:"changes"`
 CreateTrack bool `json:"createTrack,omitempty"`
}
type VocalResult struct {
 Track Track `json:"track"`
 Revision int64 `json:"revision"`
 Added int `json:"added"`
 Updated int `json:"updated"`
 Deleted int `json:"deleted"`
}

// ApplyVocalPatch is atomic: creating a missing vocal track and adding/editing
// its notes share one revision, with a concurrency guard (baseRevision).
func (s *Store) ApplyVocalPatch(p VocalPatch)(VocalResult,error){
 result:=VocalResult{}
 if strings.TrimSpace(p.Track)==""||len(p.Track)>256{return result,errors.New("vocal track name required")}
 if len(p.Changes)<1||len(p.Changes)>4096{return result,errors.New("specify 1..4096 vocal changes")}
 tx,err:=s.DB.Begin();if err!=nil{return result,err}
 defer tx.Rollback()
 head,err:=latest(tx);if err!=nil{return result,err}
 if head!=p.BaseRevision{return result,fmt.Errorf("stale vocal patch: current HEAD %d, baseRevision %d; re-inspect and retry",head,p.BaseRevision)}
 var track Track
 err=tx.QueryRow("SELECT id,name,kind,singer FROM tracks WHERE id=? OR name=?",p.Track,p.Track).Scan(&track.ID,&track.Name,&track.Kind,&track.Singer)
 changes:=[]Change{}
 if errors.Is(err,sql.ErrNoRows){
  if !p.CreateTrack{return result,fmt.Errorf("vocal track %q does not exist: pass createTrack=true to add it safely",p.Track)}
  track=Track{ID:ID(),Name:strings.TrimSpace(p.Track),Kind:"vocal",Singer:p.Singer}
  if _,err=tx.Exec("INSERT INTO tracks(id,name,kind,singer) VALUES(?,?,?,?)",track.ID,track.Name,track.Kind,track.Singer);err!=nil{return result,err}
  changes=append(changes,newChange("track",track.ID,nil,track))
 }else if err!=nil{return result,err}
 if track.Kind!="vocal"{return result,fmt.Errorf("track %q is %q, not a vocal track",track.Name,track.Kind)}
 if p.Singer!=""&&track.Singer==""{
  original:=track
  track.Singer=p.Singer
  if _,err=tx.Exec("UPDATE tracks SET singer=? WHERE id=?",track.Singer,track.ID);err!=nil{return result,err}
  changes=append(changes,newChange("track",track.ID,original,track))
 }
 visited:=map[string]bool{}
 for _,op:=range p.Changes{
  if op.Action!="add"&&op.Action!="update"&&op.Action!="delete"{return result,fmt.Errorf("unsupported vocal action %q",op.Action)}
  if op.Action=="add"{
   if op.Note==nil{return result,errors.New("add needs note")}
   if err=Validate(*op.Note);err!=nil{return result,err}
   note:=Note{ID:ID(),TrackID:track.ID,Tick:op.Note.Tick,Duration:op.Note.Duration,Pitch:op.Note.Pitch,Velocity:op.Note.Velocity,Lyric:op.Note.Lyric}
   if err=putNote(tx,note);err!=nil{return result,err}
   changes=append(changes,newChange("note",note.ID,nil,note))
   result.Added++
   continue
  }
  if op.NoteID==""{return result,errors.New("update/delete needs noteId from track_inspect")}
  if visited[op.NoteID]{return result,fmt.Errorf("note %s modified twice",op.NoteID)}
  visited[op.NoteID]=true
  var note Note
  err=tx.QueryRow("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE id=? AND track_id=?",op.NoteID,track.ID).
   Scan(&note.ID,&note.TrackID,&note.Tick,&note.Duration,&note.Pitch,&note.Velocity,&note.Lyric)
  if err!=nil{return result,fmt.Errorf("vocal note %q not found on %s: %w",op.NoteID,track.Name,err)}
  previous:=note
  if op.Action=="delete"{
   if _,err=tx.Exec("DELETE FROM notes WHERE id=?",note.ID);err!=nil{return result,err}
   changes=append(changes,newChange("note",note.ID,previous,nil))
   result.Deleted++
   continue
  }
  if op.Pitch==nil&&op.Lyric==nil&&op.Velocity==nil&&op.Tick==nil&&op.Duration==nil{return result,errors.New("update requires at least one changed field")}
  if op.Pitch!=nil{note.Pitch=*op.Pitch}
  if op.Lyric!=nil{note.Lyric=*op.Lyric}
  if op.Velocity!=nil{note.Velocity=*op.Velocity}
  if op.Tick!=nil{note.Tick=*op.Tick}
  if op.Duration!=nil{note.Duration=*op.Duration}
  if err=Validate(InputNote{Tick:note.Tick,Duration:note.Duration,Pitch:note.Pitch,Velocity:note.Velocity,Lyric:note.Lyric});err!=nil{return result,err}
  if err=putNote(tx,note);err!=nil{return result,err}
  changes=append(changes,newChange("note",note.ID,previous,note))
  result.Updated++
 }
 message:=strings.TrimSpace(p.Message)
 if message==""{message=fmt.Sprintf("Vocal edits on %s: %d added, %d updated, %d deleted",track.Name,result.Added,result.Updated,result.Deleted)}
 rev,err:=record(tx,"vocal.edit",message,changes);if err!=nil{return result,err}
 if err=tx.Commit();err!=nil{return result,err}
 result.Track=track;result.Revision=rev
 return result,nil
}
