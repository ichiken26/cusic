package store
import(
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "io"
 "os"
 "path/filepath"
 "strings"
 "database/sql"
 "encoding/json"
 "github.com/ichiken26/cusic/internal/audio"
)
type Asset struct{
 ID string `json:"id"`
 Name string `json:"name"`
 Path string `json:"path"`
 Stats audio.Stats `json:"stats"`
}
func ensureAudioSchema(db interface{Exec(string,...any)(sql.Result,error)})error{
 _,err:=db.Exec("CREATE TABLE IF NOT EXISTS audio_assets(id TEXT PRIMARY KEY,name TEXT NOT NULL,path TEXT NOT NULL,stats_json TEXT NOT NULL)")
 return err
}
func (s *Store) Assets()([]Asset,error){
 rs,err:=s.DB.Query("SELECT id,name,path,stats_json FROM audio_assets ORDER BY name,id");if err!=nil{return nil,err};defer rs.Close()
 out:=[]Asset{}
 for rs.Next(){
  var a Asset;var stats string
  if err:=rs.Scan(&a.ID,&a.Name,&a.Path,&stats);err!=nil{return nil,err}
  if err:=json.Unmarshal([]byte(stats),&a.Stats);err!=nil{return nil,err}
  out=append(out,a)
 }
 return out,rs.Err()
}
func (s *Store) AssetAbsolute(a Asset)(string,error){
 root,err:=filepath.Abs(s.Root);if err!=nil{return "",err}
 path:=filepath.Join(root,filepath.FromSlash(a.Path))
 rel,err:=filepath.Rel(root,path);if err!=nil{return "",err}
 if filepath.IsAbs(a.Path)||strings.HasPrefix(rel,".."){return "",errors.New("unsafe audio asset path")}
 return path,nil
}
func (s *Store) AddAudioFile(path,name string)(Asset,int64,error){
 stat,err:=audio.Inspect(path);if err!=nil{return Asset{},0,err}
 if stat.Samples==0{return Asset{},0,errors.New("empty audio stem")}
 name=strings.TrimSpace(name);if name==""{name=strings.TrimSuffix(filepath.Base(path),filepath.Ext(path))}
 if name==""{return Asset{},0,errors.New("stem name is empty")}
 // Assets are kept outside SQLite, indexed and hashed inside it.
 rel:=filepath.ToSlash(filepath.Join(".cusic","audio",stat.SHA256+".wav"))
 absolute:=filepath.Join(s.Root,filepath.FromSlash(rel))
 if err:=os.MkdirAll(filepath.Dir(absolute),0755);err!=nil{return Asset{},0,err}
 if _,err:=os.Stat(absolute);errors.Is(err,os.ErrNotExist){
  src,err:=os.Open(path);if err!=nil{return Asset{},0,err};defer src.Close()
  tmp,err:=os.CreateTemp(filepath.Dir(absolute),".cusic-audio-*");if err!=nil{return Asset{},0,err}
  defer os.Remove(tmp.Name())
  hash:=sha256.New()
  if _,err=io.Copy(io.MultiWriter(tmp,hash),src);err!=nil{tmp.Close();return Asset{},0,err}
  if hex.EncodeToString(hash.Sum(nil))!=stat.SHA256{tmp.Close();return Asset{},0,errors.New("stem changed while being imported")}
  if err=tmp.Close();err!=nil{return Asset{},0,err}
  if err=os.Rename(tmp.Name(),absolute);err!=nil{return Asset{},0,err}
 }else if err!=nil{return Asset{},0,err}
 a:=Asset{ID:ID(),Name:name,Path:rel,Stats:stat}
 tx,err:=s.DB.Begin();if err!=nil{return a,0,err};defer tx.Rollback()
 if err=putAsset(tx,a);err!=nil{return a,0,err}
 rev,err:=record(tx,"audio.import","Import stem "+name,[]Change{newChange("asset",a.ID,nil,a)})
 if err!=nil{return a,0,err};return a,rev,tx.Commit()
}
func putAsset(tx *sql.Tx,a Asset)error{
 b,err:=json.Marshal(a.Stats);if err!=nil{return err}
 _,err=tx.Exec("INSERT INTO audio_assets(id,name,path,stats_json) VALUES(?,?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name,path=excluded.path,stats_json=excluded.stats_json",a.ID,a.Name,a.Path,string(b))
 return err
}
