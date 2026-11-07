package store

import (
 "crypto/rand"
 "database/sql"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "time"

 _ "modernc.org/sqlite"
)

const PPQ int64 = 480

type Track struct {
 ID string `json:"id"`
 Name string `json:"name"`
 Kind string `json:"kind"`
 Singer string `json:"singer"`
}
type Note struct {
 ID string `json:"id"`
 TrackID string `json:"trackId"`
 Tick int64 `json:"tick"`
 Duration int64 `json:"duration"`
 Pitch int `json:"pitch"`
 Velocity int `json:"velocity"`
 Lyric string `json:"lyric"`
}
type InputNote struct {
 Tick int64 `json:"tick"`
 Duration int64 `json:"duration"`
 Pitch int `json:"pitch"`
 Velocity int `json:"velocity"`
 Lyric string `json:"lyric"`
}
type Patch struct {
 BaseRevision int64 `json:"baseRevision"`
 Message string `json:"message"`
 Track string `json:"track"`
 FromBar int `json:"fromBar"`
 ToBar int `json:"toBar"`
 Notes []InputNote `json:"notes"`
}
type Change struct {
 Entity string `json:"entity"`
 ID string `json:"id"`
 Before json.RawMessage `json:"before"`
 After json.RawMessage `json:"after"`
}
type Revision struct {
 ID int64 `json:"id"`
 At string `json:"at"`
 Operation string `json:"operation"`
 Message string `json:"message"`
 Changes []Change `json:"changes"`
}
type Settings struct {
 Name string `json:"name"`
 Tempo int `json:"tempo"`
 MeterNum int `json:"meterNum"`
 MeterDen int `json:"meterDen"`
 PPQ int64 `json:"ppq"`
}
type Store struct { DB *sql.DB; Root string }

func ID() string {
 b:=make([]byte,16); if _,err:=rand.Read(b);err!=nil { panic(err) }
 return hex.EncodeToString(b)
}
func DBPath(dir string) string { return filepath.Join(dir,".cusic","project.db") }
func FindRoot(start string)(string,error){
 dir,err:=filepath.Abs(start); if err!=nil{return "",err}
 for {
  if st,e:=os.Stat(DBPath(dir));e==nil && !st.IsDir(){return dir,nil}
  parent:=filepath.Dir(dir);if parent==dir{return "",errors.New("not a cusic project; run cusic init first")};dir=parent
 }
}
func Init(dir string, s Settings) error {
 if s.Name=="" {return errors.New("name required")}
 if s.Tempo<20 || s.Tempo>400 {return errors.New("tempo must be 20..400")}
 if s.MeterNum<1 || s.MeterNum>32 || (s.MeterDen!=2 && s.MeterDen!=4 && s.MeterDen!=8 && s.MeterDen!=16) {return errors.New("unsupported meter")}
 if _,err:=os.Stat(DBPath(dir));err==nil{return errors.New("project already exists")}
 if err:=os.MkdirAll(filepath.Dir(DBPath(dir)),0755);err!=nil{return err}
 db,err:=sql.Open("sqlite",DBPath(dir));if err!=nil{return err}
 defer db.Close()
 if err:=configure(db);err!=nil{return err}
 if err:=schema(db);err!=nil{return err}
 s.PPQ=PPQ
 b,_:=json.Marshal(s)
 _,err=db.Exec("INSERT INTO settings(key,value) VALUES('project',?)",string(b))
 return err
}
func Open(dir string)(*Store,error){
 root,err:=FindRoot(dir);if err!=nil{return nil,err}
 db,err:=sql.Open("sqlite",DBPath(root));if err!=nil{return nil,err}
 db.SetMaxOpenConns(1)
 if err:=configure(db);err!=nil {db.Close();return nil,err}
 return &Store{DB:db,Root:root},nil
}
func configure(db *sql.DB)error{
 for _,q:=range []string{"PRAGMA foreign_keys=ON","PRAGMA busy_timeout=5000","PRAGMA journal_mode=WAL"} {
  if _,err:=db.Exec(q);err!=nil{return err}
 }
 return nil
}
func schema(db *sql.DB)error{
 for _,q:=range []string{
  "CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT NOT NULL)",
  "CREATE TABLE tracks(id TEXT PRIMARY KEY,name TEXT NOT NULL UNIQUE,kind TEXT NOT NULL,singer TEXT NOT NULL DEFAULT '')",
  "CREATE TABLE notes(id TEXT PRIMARY KEY,track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,tick INTEGER NOT NULL CHECK(tick>=0),duration INTEGER NOT NULL CHECK(duration>0),pitch INTEGER NOT NULL CHECK(pitch BETWEEN 0 AND 127),velocity INTEGER NOT NULL CHECK(velocity BETWEEN 1 AND 127),lyric TEXT NOT NULL DEFAULT '')",
  "CREATE INDEX notes_by_track_tick ON notes(track_id,tick)",
  "CREATE TABLE revisions(id INTEGER PRIMARY KEY AUTOINCREMENT,at TEXT NOT NULL,operation TEXT NOT NULL,message TEXT NOT NULL,changes_json TEXT NOT NULL)",
 }{if _,err:=db.Exec(q);err!=nil{return fmt.Errorf("migration: %w",err)}}
 return nil
}
func (s *Store) Close()error{return s.DB.Close()}
func (s *Store) Settings()(Settings,error){
 var b string;var v Settings
 err:=s.DB.QueryRow("SELECT value FROM settings WHERE key='project'").Scan(&b)
 if err!=nil{return v,err}
 err=json.Unmarshal([]byte(b),&v);return v,err
}
func (s *Store) Tracks()([]Track,error){
 rs,err:=s.DB.Query("SELECT id,name,kind,singer FROM tracks ORDER BY rowid");if err!=nil{return nil,err};defer rs.Close()
 out:=[]Track{}
 for rs.Next(){var v Track;if err:=rs.Scan(&v.ID,&v.Name,&v.Kind,&v.Singer);err!=nil{return nil,err};out=append(out,v)}
 return out,rs.Err()
}
func (s *Store) Track(ref string)(Track,error){
 var t Track
 err:=s.DB.QueryRow("SELECT id,name,kind,singer FROM tracks WHERE id=? OR name=?",ref,ref).Scan(&t.ID,&t.Name,&t.Kind,&t.Singer)
 return t,err
}
func (s *Store) Notes(trackID string,from,to int64)([]Note,error){
 var rs *sql.Rows;var err error
 if to>from {rs,err=s.DB.Query("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE track_id=? AND tick< ? AND tick+duration> ? ORDER BY tick,pitch,id",trackID,to,from)
 }else{rs,err=s.DB.Query("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE track_id=? ORDER BY tick,pitch,id",trackID)}
 if err!=nil{return nil,err};defer rs.Close()
 out:=[]Note{}
 for rs.Next(){var n Note;if err:=rs.Scan(&n.ID,&n.TrackID,&n.Tick,&n.Duration,&n.Pitch,&n.Velocity,&n.Lyric);err!=nil{return nil,err};out=append(out,n)}
 return out,rs.Err()
}
func Validate(n InputNote)error{
 if n.Tick<0||n.Duration<1||n.Tick>1<<55||n.Duration>1<<55||n.Tick+n.Duration>1<<56{return errors.New("invalid tick/duration")}
 if n.Pitch<0||n.Pitch>127||n.Velocity<1||n.Velocity>127{return errors.New("pitch must be 0..127 and velocity 1..127")}
 return nil
}
func raw(v any) json.RawMessage {if v==nil{return nil};b,err:=json.Marshal(v);if err!=nil{panic(err)};return b}
func newChange(typ,id string,before,after any) Change{return Change{Entity:typ,ID:id,Before:raw(before),After:raw(after)}}
func latest(tx *sql.Tx)(int64,error){var id int64;err:=tx.QueryRow("SELECT COALESCE(MAX(id),0) FROM revisions").Scan(&id);return id,err}
func record(tx *sql.Tx, op,msg string, changes []Change)(int64,error){
 b,err:=json.Marshal(changes);if err!=nil{return 0,err}
 r,err:=tx.Exec("INSERT INTO revisions(at,operation,message,changes_json) VALUES(?,?,?,?)",time.Now().UTC().Format(time.RFC3339),op,msg,string(b));if err!=nil{return 0,err};return r.LastInsertId()
}
func (s *Store) AddTrack(name,kind,singer string)(Track,int64,error){
 name=strings.TrimSpace(name);if name=="" {return Track{},0,errors.New("track name required")}
 if kind==""{kind="instrument"}
 if kind!="instrument"&&kind!="vocal"&&kind!="drums"{return Track{},0,errors.New("kind must be instrument, drums or vocal")}
 t:=Track{ID:ID(),Name:name,Kind:kind,Singer:singer}
 tx,err:=s.DB.Begin();if err!=nil{return t,0,err};defer tx.Rollback()
 if _,err=tx.Exec("INSERT INTO tracks(id,name,kind,singer) VALUES(?,?,?,?)",t.ID,t.Name,t.Kind,t.Singer);err!=nil{return t,0,err}
 rev,err:=record(tx,"track.add","Add track "+name,[]Change{newChange("track",t.ID,nil,t)})
 if err!=nil{return t,0,err};return t,rev,tx.Commit()
}
func (s *Store) AddNote(trackRef string,n InputNote)(Note,int64,error){
 if err:=Validate(n);err!=nil{return Note{},0,err}
 t,err:=s.Track(trackRef);if err!=nil{return Note{},0,err}
 a:=Note{ID:ID(),TrackID:t.ID,Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Velocity:n.Velocity,Lyric:n.Lyric}
 tx,err:=s.DB.Begin();if err!=nil{return a,0,err};defer tx.Rollback()
 if err:=putNote(tx,a);err!=nil{return a,0,err}
 rev,err:=record(tx,"note.add","Add note to "+t.Name,[]Change{newChange("note",a.ID,nil,a)})
 if err!=nil{return a,0,err};return a,rev,tx.Commit()
}
func putNote(tx *sql.Tx,n Note)error{
 _,err:=tx.Exec("INSERT INTO notes(id,track_id,tick,duration,pitch,velocity,lyric) VALUES(?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET track_id=excluded.track_id,tick=excluded.tick,duration=excluded.duration,pitch=excluded.pitch,velocity=excluded.velocity,lyric=excluded.lyric",n.ID,n.TrackID,n.Tick,n.Duration,n.Pitch,n.Velocity,n.Lyric);return err
}
func putTrack(tx *sql.Tx,t Track)error{
 _,err:=tx.Exec("INSERT INTO tracks(id,name,kind,singer) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,kind=excluded.kind,singer=excluded.singer",t.ID,t.Name,t.Kind,t.Singer);return err
}
func (s *Store) ApplyPatch(p Patch)(int64,error){
 if p.FromBar<1||p.ToBar<p.FromBar{return 0,errors.New("invalid inclusive bar range")}
 settings,err:=s.Settings();if err!=nil{return 0,err}
 barTicks:=int64(settings.MeterNum)*PPQ*4/int64(settings.MeterDen)
 from,to:=int64(p.FromBar-1)*barTicks,int64(p.ToBar)*barTicks
 for _,n:=range p.Notes{if err:=Validate(n);err!=nil{return 0,err};if n.Tick<from||n.Tick+n.Duration>to{return 0,fmt.Errorf("note at %d goes beyond patch range [%d,%d)",n.Tick,from,to)}}
 t,err:=s.Track(p.Track);if err!=nil{return 0,err}
 tx,err:=s.DB.Begin();if err!=nil{return 0,err};defer tx.Rollback()
 current,err:=latest(tx);if err!=nil{return 0,err}
 if current!=p.BaseRevision{return 0,fmt.Errorf("stale patch: expected revision %d, got %d; inspect and retry",current,p.BaseRevision)}
 rs,err:=tx.Query("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE track_id=? AND tick< ? AND tick+duration> ? ORDER BY tick,id",t.ID,to,from)
 if err!=nil{return 0,err}
 previous:=[]Note{}
 for rs.Next(){var n Note;if err=rs.Scan(&n.ID,&n.TrackID,&n.Tick,&n.Duration,&n.Pitch,&n.Velocity,&n.Lyric);err!=nil{rs.Close();return 0,err};previous=append(previous,n)}
 err=rs.Err();rs.Close();if err!=nil{return 0,err}
 for _,n:=range previous{if n.Tick<from||n.Tick+n.Duration>to{return 0,fmt.Errorf("note %s crosses edit boundary; split it first",n.ID)}}
 changes:=[]Change{}
 for _,n:=range previous{if _,err=tx.Exec("DELETE FROM notes WHERE id=?",n.ID);err!=nil{return 0,err};changes=append(changes,newChange("note",n.ID,n,nil))}
 for _,input:=range p.Notes{
  n:=Note{ID:ID(),TrackID:t.ID,Tick:input.Tick,Duration:input.Duration,Pitch:input.Pitch,Velocity:input.Velocity,Lyric:input.Lyric}
  if err=putNote(tx,n);err!=nil{return 0,err}
  changes=append(changes,newChange("note",n.ID,nil,n))
 }
 msg:=strings.TrimSpace(p.Message);if msg==""{msg=fmt.Sprintf("Replace %s bars %d-%d",t.Name,p.FromBar,p.ToBar)}
 rev,err:=record(tx,"notes.replace",msg,changes);if err!=nil{return 0,err};return rev,tx.Commit()
}
func (s *Store) Revisions(limit int)([]Revision,error){
 if limit<1||limit>1000{limit=20}
 rs,err:=s.DB.Query("SELECT id,at,operation,message,changes_json FROM revisions ORDER BY id DESC LIMIT ?",limit);if err!=nil{return nil,err};defer rs.Close()
 out:=[]Revision{}
 for rs.Next(){var r Revision;var raw string;if err:=rs.Scan(&r.ID,&r.At,&r.Operation,&r.Message,&raw);err!=nil{return nil,err};if err:=json.Unmarshal([]byte(raw),&r.Changes);err!=nil{return nil,err};out=append(out,r)}
 return out,rs.Err()
}
func (s *Store) Revision(id int64)(Revision,error){
 var r Revision;var raw string
 err:=s.DB.QueryRow("SELECT id,at,operation,message,changes_json FROM revisions WHERE id=?",id).Scan(&r.ID,&r.At,&r.Operation,&r.Message,&raw);if err!=nil{return r,err}
 err=json.Unmarshal([]byte(raw),&r.Changes);return r,err
}
func equalJSON(a,b json.RawMessage)bool{
 if len(a)==0&&len(b)==0{return true};var x,y any
 if json.Unmarshal(a,&x)!=nil||json.Unmarshal(b,&y)!=nil{return false}
 return reflect.DeepEqual(x,y)
}
func currentState(tx *sql.Tx,c Change)(json.RawMessage,error){
 switch c.Entity{
 case "note":
  var n Note
  err:=tx.QueryRow("SELECT id,track_id,tick,duration,pitch,velocity,lyric FROM notes WHERE id=?",c.ID).Scan(&n.ID,&n.TrackID,&n.Tick,&n.Duration,&n.Pitch,&n.Velocity,&n.Lyric)
  if errors.Is(err,sql.ErrNoRows){return nil,nil};if err!=nil{return nil,err};return raw(n),nil
 case "track":
  var t Track
  err:=tx.QueryRow("SELECT id,name,kind,singer FROM tracks WHERE id=?",c.ID).Scan(&t.ID,&t.Name,&t.Kind,&t.Singer)
  if errors.Is(err,sql.ErrNoRows){return nil,nil};if err!=nil{return nil,err};return raw(t),nil
 }
 return nil,fmt.Errorf("unknown entity %s",c.Entity)
}
func (s *Store) Revert(id int64)(int64,error){
 tx,err:=s.DB.Begin();if err!=nil{return 0,err};defer tx.Rollback()
 current,err:=latest(tx);if err!=nil{return 0,err}
 if current!=id{return 0,fmt.Errorf("only HEAD (revision %d) can be reverted safely",current)}
 var op,msg,b string
 if err=tx.QueryRow("SELECT operation,message,changes_json FROM revisions WHERE id=?",id).Scan(&op,&msg,&b);err!=nil{return 0,err}
 var cs []Change;if err=json.Unmarshal([]byte(b),&cs);err!=nil{return 0,err}
 for _,c:=range cs{state,e:=currentState(tx,c);if e!=nil{return 0,e};if !equalJSON(state,c.After){return 0,fmt.Errorf("%s %s was modified outside cusic; refusing revert",c.Entity,c.ID)}}
 // Delete notes before tracks, then upsert tracks before notes (foreign keys).
 for _,typ:=range []string{"note","track"} {
  for _,c:=range cs{if c.Entity!=typ||len(c.Before)>0{continue}
   table:="notes";if typ=="track"{table="tracks"}
   if _,err=tx.Exec("DELETE FROM "+table+" WHERE id=?",c.ID);err!=nil{return 0,err}
  }
 }
 for _,typ:=range []string{"track","note"}{
  for _,c:=range cs{if c.Entity!=typ||len(c.Before)==0{continue}
   if typ=="track"{var t Track;if err=json.Unmarshal(c.Before,&t);err!=nil{return 0,err};err=putTrack(tx,t)
   }else{var n Note;if err=json.Unmarshal(c.Before,&n);err!=nil{return 0,err};err=putNote(tx,n)}
   if err!=nil{return 0,err}
  }
 }
 inverse:=make([]Change,len(cs));for i,c:=range cs{inverse[i]=Change{Entity:c.Entity,ID:c.ID,Before:c.After,After:c.Before}}
 rev,err:=record(tx,"revert",fmt.Sprintf("Revert #%d: %s",id,msg),inverse)
 if err!=nil{return 0,err};return rev,tx.Commit()
}
func (s *Store) AddImported(tracks []ImportedTrack,tempo,meterNum,meterDen int)(int64,error){
 if len(tracks)==0{return 0,errors.New("no MIDI notes found")}
 settings,err:=s.Settings();if err!=nil{return 0,err}
 // Reject incompatible timing: avoid silently shifting the existing project.
 if count,_:=s.DB.Query("SELECT 1 FROM tracks LIMIT 1");count!=nil{if count.Next(){count.Close();if settings.Tempo!=tempo||settings.MeterNum!=meterNum||settings.MeterDen!=meterDen{return 0,errors.New("MIDI tempo/meter mismatch with project; import into a matching project")}}else{count.Close()}}
 tx,err:=s.DB.Begin();if err!=nil{return 0,err};defer tx.Rollback()
 var existing int;if err=tx.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&existing);err!=nil{return 0,err}
 if existing==0 {
  settings.Tempo,settings.MeterNum,settings.MeterDen=tempo,meterNum,meterDen
  b,_:=json.Marshal(settings);if _,err=tx.Exec("UPDATE settings SET value=? WHERE key='project'",string(b));err!=nil{return 0,err}
 }
 cs:=[]Change{}
 for _,src:=range tracks{
  t:=Track{ID:ID(),Name:src.Name,Kind:src.Kind}
  if t.Kind==""{t.Kind="instrument"}
  name:=t.Name;if name==""{name="Imported"}
  // Make imported track names unique.
  for n:=0;;n++{
   if n==0{t.Name=name}else{t.Name=fmt.Sprintf("%s (%d)",name,n+1)}
   var dummy int;e:=tx.QueryRow("SELECT 1 FROM tracks WHERE name=?",t.Name).Scan(&dummy)
   if errors.Is(e,sql.ErrNoRows){break};if e!=nil{return 0,e}
  }
  if err=putTrack(tx,t);err!=nil{return 0,err}
  cs=append(cs,newChange("track",t.ID,nil,t))
  for _,input:=range src.Notes{
   if err=Validate(input);err!=nil{return 0,err}
   n:=Note{ID:ID(),TrackID:t.ID,Tick:input.Tick,Duration:input.Duration,Pitch:input.Pitch,Velocity:input.Velocity,Lyric:input.Lyric}
   if err=putNote(tx,n);err!=nil{return 0,err}
   cs=append(cs,newChange("note",n.ID,nil,n))
  }
 }
 rev,err:=record(tx,"midi.import","Import MIDI",cs);if err!=nil{return 0,err};return rev,tx.Commit()
}
type ImportedTrack struct{Name string;Kind string;Notes []InputNote}
