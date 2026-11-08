// Package agent exposes cusic's local project as a stdio MCP server.
// All tools are project-scoped. stdout is reserved for MCP JSON-RPC.
package agent

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "io"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 "github.com/ichiken26/cusic/internal/audio"
 "github.com/ichiken26/cusic/internal/dawproject"
 "github.com/ichiken26/cusic/internal/midifile"
 "github.com/ichiken26/cusic/internal/mix"
 "github.com/ichiken26/cusic/internal/store"
 "github.com/ichiken26/cusic/internal/tuxguitar"
 "github.com/ichiken26/cusic/internal/vsqx"
 "github.com/modelcontextprotocol/go-sdk/mcp"
)
type response struct{ Result string `json:"result"` }
func output(value any)(*mcp.CallToolResult,response,error){
 b,err:=json.MarshalIndent(value,"","  ");if err!=nil{return nil,response{},err}
 return nil,response{Result:string(b)},nil
}
func projectDir()string{
 if p:=os.Getenv("CUSIC_PROJECT");p!=""{return p}
 cwd,_:=os.Getwd();return cwd
}
func withStore(fn func(*store.Store)(any,error))(*mcp.CallToolResult,response,error){
 s,err:=store.Open(projectDir());if err!=nil{return nil,response{},fmt.Errorf("%w: ask the user to export their existing Cubase project as DAWproject or initialize cusic in this folder",err)}
 defer s.Close()
 v,err:=fn(s);if err!=nil{return nil,response{},err}
 return output(v)
}
func pathFromProject(path string)string{
 if filepath.IsAbs(path){return filepath.Clean(path)}
 return filepath.Join(projectDir(),path)
}
func outputPath(path string)(string,error){
 if path==""{return "",errors.New("output path required")}
 root,err:=filepath.Abs(projectDir());if err!=nil{return "",err}
 file,err:=filepath.Abs(pathFromProject(path));if err!=nil{return "",err}
 rel,err:=filepath.Rel(root,file);if err!=nil{return "",err}
 if rel=="."||rel==".."||strings.HasPrefix(rel,".."+string(os.PathSeparator)){return "",errors.New("output must be within the active cusic project")}
 if _,err:=os.Stat(file);err==nil{return "",errors.New("output already exists; use a new filename")}
 return file,nil
}
type empty struct{}
type trackRef struct{ Track string `json:"track" jsonschema:"track name or ID"` }
type inspectInput struct{ Track string `json:"track"`; FromBar int `json:"fromBar"`; ToBar int `json:"toBar"` }
type initInput struct{
 Name string `json:"name"`
 Tempo int `json:"tempo"`
 MeterNum int `json:"meterNum"`
 MeterDen int `json:"meterDen"`
}
type newTrack struct{ Name string `json:"name"`; Kind string `json:"kind"`; Singer string `json:"singer"` }
type importInput struct{ File string `json:"file"` }
type patchInput struct{
 BaseRevision int64 `json:"baseRevision"`
 Message string `json:"message"`
 Track string `json:"track"`
 FromBar int `json:"fromBar"`
 ToBar int `json:"toBar"`
 Notes []store.InputNote `json:"notes"`
}
type revisionInput struct{ Revision int64 `json:"revision"` }
type vsqxEditsInput struct{ Source string `json:"source"`; Out string `json:"out"`; Changes []vsqx.NoteEdit `json:"changes"` }
type audioInput struct{ File string `json:"file"`; Name string `json:"name"` }
type vocalSourceInput struct{ Source string `json:"source"` }
type exportInput struct{
 Format string `json:"format"`
 Out string `json:"out"`
 Track string `json:"track"`
 Template string `json:"template"`
}
type syncInput struct{ From string `json:"from"`; BaseRevision int64 `json:"baseRevision"`; Confirm bool `json:"confirm"` }
type mixInput struct{ Source string `json:"source"`; Out string `json:"out"`; Tracks []mix.Track `json:"tracks"`; Confirm bool `json:"confirm"` }
func NewServer()*mcp.Server{
 server:=mcp.NewServer(&mcp.Implementation{Name:"cusic",Version:"0.3.0"},nil)
 mcp.AddTool(server,&mcp.Tool{Name:"project_inventory",Description:"Discover Cubase .cpr, DAWproject, MIDI, WAV, VSQX and SVP files in this directory. No modifications."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  root:=projectDir()
  files:=map[string][]string{".cpr":{}, ".dawproject":{}, ".mid":{}, ".midi":{}, ".tg":{}, ".wav":{}, ".vsqx":{}, ".svp":{}}
  err:=filepath.WalkDir(root,func(path string,e os.DirEntry,visitErr error)error{
   if visitErr!=nil{return visitErr}
   rel,err:=filepath.Rel(root,path);if err!=nil{return err}
   if e.IsDir(){if rel!="."&&(strings.HasPrefix(e.Name(),".")||e.Name()=="node_modules"||e.Name()=="Audio"){return filepath.SkipDir};return nil}
   ext:=strings.ToLower(filepath.Ext(path));if _,ok:=files[ext];ok{files[ext]=append(files[ext],rel)}
   return nil
  })
  if err!=nil{return nil,response{},err}
  return output(map[string]any{"root":root,"files":files,"hint":"A .cpr is not read directly. Export a .dawproject from Cubase once; then use project_import. Vocal originals .vsqx/.svp should be backed up before editing."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"project_initialize",Description:"Create a new per-song SQLite project in this directory. Call only when no project exists, with confirmed tempo and meter."},func(ctx context.Context,req *mcp.CallToolRequest,in initInput)(*mcp.CallToolResult,response,error){
  err:=store.Init(projectDir(),store.Settings{Name:in.Name,Tempo:in.Tempo,MeterNum:in.MeterNum,MeterDen:in.MeterDen})
  if err!=nil{return nil,response{},err};return output(map[string]any{"initialized":true,"directory":projectDir()})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"project_status",Description:"Read project tempo, time signature, SQLite HEAD revision and available tracks. Run first before any edits."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   cfg,err:=s.Settings();if err!=nil{return nil,err}
   tracks,err:=s.Tracks();if err!=nil{return nil,err}
   revs,err:=s.Revisions(1);if err!=nil{return nil,err}
   head:=int64(0);if len(revs)>0{head=revs[0].ID}
   return map[string]any{"project":cfg,"tracks":tracks,"head":head},nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"track_create",Description:"Add an instrument, drum or vocal track. Existing tracks remain unchanged."},func(ctx context.Context,req *mcp.CallToolRequest,in newTrack)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){t,r,e:=s.AddTrack(in.Name,in.Kind,in.Singer);return map[string]any{"track":t,"revision":r},e})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"track_inspect",Description:"Read notes from the selected track in a 1-based inclusive bar interval; use to understand arrangements before patching."},func(ctx context.Context,req *mcp.CallToolRequest,in inspectInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   t,err:=s.Track(in.Track);if err!=nil{return nil,err};cfg,err:=s.Settings();if err!=nil{return nil,err}
   from,to:=int64(0),int64(0)
   if in.FromBar!=0||in.ToBar!=0{
    if in.FromBar<1||in.ToBar<in.FromBar{return nil,errors.New("invalid inclusive bar range")}
    ticks:=int64(cfg.MeterNum)*store.PPQ*4/int64(cfg.MeterDen)
    from,to=int64(in.FromBar-1)*ticks,int64(in.ToBar)*ticks
   }
   ns,err:=s.Notes(t.ID,from,to);return map[string]any{"track":t,"notes":ns,"ppq":store.PPQ},err
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"project_import",Description:"Import a .mid, TuxGuitar 2.x .tg or Cubase .dawproject file. For DAWproject also register embedded WAV stems. Does not modify the original."},func(ctx context.Context,req *mcp.CallToolRequest,in importInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   file:=pathFromProject(in.File);var song midifile.Song;var aud []dawproject.ImportedAudio;var err error
   switch strings.ToLower(filepath.Ext(file)){
   case ".mid",".midi":song,err=midifile.ReadFile(file)
   case ".tg":song,err=tuxguitar.ReadFile(file)
   case ".dawproject":
    var imported dawproject.Imported;imported,err=dawproject.ReadFile(file);song=imported.Song;aud=imported.Audio
   default:return nil,errors.New("supported import formats: .mid, .midi, .tg (2.x), .dawproject")
   }
   if err!=nil{return nil,err}
   revisions:=[]int64{}
   if len(song.Tracks)>0{r,e:=s.AddImported(song.Tracks,song.Tempo,song.MeterNum,song.MeterDen);if e!=nil{return nil,e};revisions=append(revisions,r)}
   for _,a:=range aud{
    tmp,e:=os.MkdirTemp("","cusic-audio-");if e!=nil{return nil,e}
    extracted:=filepath.Join(tmp,"audio.wav")
    e=dawproject.ExtractAudio(file,a.ArchivePath,extracted)
    if e==nil{_,rev,addErr:=s.AddAudioFile(extracted,a.Name);e=addErr;if e==nil{revisions=append(revisions,rev)}}
    os.RemoveAll(tmp)
    if e!=nil{return nil,e}
   }
   return map[string]any{"tracksImported":len(song.Tracks),"audioImported":len(aud),"revisions":revisions,"caution":"MIDI CC, VST state and full vocal tuning are not imported into the note model"},nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"audio_import",Description:"Import and fingerprint an existing WAV stem as an immutable external asset, without changing the original WAV."},func(ctx context.Context,req *mcp.CallToolRequest,in audioInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){a,r,e:=s.AddAudioFile(pathFromProject(in.File),in.Name);return map[string]any{"asset":a,"revision":r},e})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"audio_list",Description:"List registered WAV stems with peak, RMS, channel count, sample rate and duration."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){return s.Assets()})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"mix_analyze_stems",Description:"Compute measured WAV peak, RMS and approximate broad-band energy for each registered stem. Use this to suggest EQ, compression and gain; it does NOT control Cubase plug-ins or automatically change the mix."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   assets,err:=s.Assets();if err!=nil{return nil,err}
   results:=[]map[string]any{}
   for _,a:=range assets{
    file,e:=s.AssetAbsolute(a);if e!=nil{return nil,e}
    stats,e:=audio.Inspect(file);if e!=nil{return nil,e}
    results=append(results,map[string]any{"name":a.Name,"stats":stats})
   }
   return map[string]any{"stems":results,"note":"Approximate broad-band RMS, not perceptual masking or LUFS. Choose EQ by listening, do not auto-write VST state."},nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"patch_apply" ,Description:"REPLACES every note in a bounded track/bar interval. Always inspect notes and current HEAD first. Requires baseRevision to match HEAD. Never automatically edit vocal tracks containing existing tuning."},func(ctx context.Context,req *mcp.CallToolRequest,in patchInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   track,err:=s.Track(in.Track);if err!=nil{return nil,err}
   if track.Kind=="vocal"{return nil,errors.New("refusing destructive vocal patch: preserve tuned vocal source and use an additive harmony or non-destructive note operation")}
   if in.Notes==nil{return nil,errors.New("notes must be a non-null array; empty array deletes region")}
   r,err:=s.ApplyPatch(store.Patch{BaseRevision:in.BaseRevision,Message:in.Message,Track:in.Track,FromBar:in.FromBar,ToBar:in.ToBar,Notes:in.Notes})
   return map[string]any{"revision":r,"track":in.Track,"bars":fmt.Sprintf("%d:%d",in.FromBar,in.ToBar)},err
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"history",Description:"Read SQLite revision history and track previous changes."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){return s.Revisions(20)})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"history_revert_head",Description:"Undo only the current HEAD revision, creating an inverse revision. Requires revision equal to current HEAD."},func(ctx context.Context,req *mcp.CallToolRequest,in revisionInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){r,e:=s.Revert(in.Revision);return map[string]any{"newHead":r},e})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"score_export",Description:"Export MIDI, DAWproject or VSQX into a NEW file under this project. Does not modify .cpr, mixer, or original vocal sources. VSQX with vocal tuning is treated as experimental; use only for new parts."},func(ctx context.Context,req *mcp.CallToolRequest,in exportInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   file,err:=outputPath(in.Out);if err!=nil{return nil,err}
   cfg,err:=s.Settings();if err!=nil{return nil,err}
   tracks,err:=s.Tracks();if err!=nil{return nil,err}
   ns:=map[string][]store.Note{};for _,t:=range tracks{n,e:=s.Notes(t.ID,0,0);if e!=nil{return nil,e};ns[t.ID]=n}
   if err:=os.MkdirAll(filepath.Dir(file),0755);err!=nil{return nil,err}
   switch in.Format{
   case "midi":
    b,e:=midifile.Write(cfg,tracks,ns);if e!=nil{return nil,e}
    return map[string]any{"path":file,"kind":"midi"},os.WriteFile(file,b,0644)
   case "dawproject":
    assets,e:=s.Assets();if e!=nil{return nil,e}
    waveFiles:=[]dawproject.AudioFile{}
    for _,a:=range assets{
     abs,e:=s.AssetAbsolute(a);if e!=nil{return nil,e}
     waveFiles=append(waveFiles,dawproject.AudioFile{Name:a.Name,AbsolutePath:abs,SampleRate:a.Stats.SampleRate,Channels:a.Stats.Channels,Duration:a.Stats.Duration})
    }
    e=dawproject.WriteFile(file,cfg,tracks,ns,waveFiles);return map[string]any{"path":file,"kind":"dawproject"},e
   case "vsqx":
    if in.Track==""||in.Template==""{return nil,errors.New("new VSQX generation needs track and a real Piapro template")}
    t,e:=s.Track(in.Track);if e!=nil{return nil,e}
    if t.Kind!="vocal"{return nil,errors.New("VSQX track must be vocal")}
    tpl,e:=os.ReadFile(pathFromProject(in.Template));if e!=nil{return nil,e}
    b,e:=vsqx.Export(tpl,cfg,t,ns[t.ID]);if e!=nil{return nil,e}
    return map[string]any{"path":file,"kind":"vsqx","warning":"Use for NEW vocal phrases only, not as a replacement for previously tuned VSQX"},os.WriteFile(file,b,0644)
   }
   return nil,errors.New("formats: midi, dawproject, vsqx")
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"cubase_sync_plan",Description:"Read a Cubase-exported DAWproject and preview note changes; vocal tracks are skipped. Does not modify SQLite."},func(ctx context.Context,req *mcp.CallToolRequest,in importInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   data,err:=dawproject.ReadFile(pathFromProject(in.File));if err!=nil{return nil,err}
   cfg,err:=s.Settings();if err!=nil{return nil,err}
   if cfg.Tempo!=data.Song.Tempo||cfg.MeterNum!=data.Song.MeterNum||cfg.MeterDen!=data.Song.MeterDen{return nil,errors.New("tempo or meter differs")}
   return s.PreviewSync(data.Song.Tracks)
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"cubase_sync_apply",Description:"Apply explicit sync from Cubase DAWproject for existing non-vocal tracks only. Requires confirm=true and baseRevision from cubase_sync_plan."},func(ctx context.Context,req *mcp.CallToolRequest,in syncInput)(*mcp.CallToolResult,response,error){
  if !in.Confirm{return nil,response{},errors.New("confirm=true required after reviewing sync plan")}
  return withStore(func(s *store.Store)(any,error){
   data,e:=dawproject.ReadFile(pathFromProject(in.From));if e!=nil{return nil,e}
   cfg,e:=s.Settings();if e!=nil{return nil,e}
   if cfg.Tempo!=data.Song.Tempo||cfg.MeterNum!=data.Song.MeterNum||cfg.MeterDen!=data.Song.MeterDen{return nil,errors.New("tempo/meter mismatch")}
   r,e:=s.SyncTracks(data.Song.Tracks,in.BaseRevision);return map[string]any{"revision":r},e
  })
 })


 mcp.AddTool(server,&mcp.Tool{Name:"mix_plan",Description:"Validate a proposed EQ/fader/pan mix for an existing Cubase-exported DAWproject; returns exact track changes without touching project files. Use after mix_analyze_stems."},func(ctx context.Context,req *mcp.CallToolRequest,in mixInput)(*mcp.CallToolResult,response,error){
  if in.Source==""{return nil,response{},errors.New("source DAWproject required")}
  p:=mix.Plan{Tracks:in.Tracks}
  if err:=mix.Check(pathFromProject(in.Source),p);err!=nil{return nil,response{},err}
  return output(map[string]any{"valid":true,"trackChanges":p.Tracks,"notice":"EQ is appended as a generic DAWproject equalizer. This is a NEW copied project, never automatic editing of open Cubase.","next":"Review with the user, then call mix_apply with confirm=true"})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"mix_apply",Description:"After approval, create a NEW DAWproject with changed channel fader/pan and added generic EQ bands. Existing VST state/WAV media in the archive are copied unchanged. Requires confirm=true."},func(ctx context.Context,req *mcp.CallToolRequest,in mixInput)(*mcp.CallToolResult,response,error){
  if !in.Confirm{return nil,response{},errors.New("explicit confirm=true required after mix_plan")}
  out,err:=outputPath(in.Out);if err!=nil{return nil,response{},err}
  source:=pathFromProject(in.Source)
  if err=mix.Apply(source,out,mix.Plan{Tracks:in.Tracks});err!=nil{return nil,response{},err}
  return output(map[string]any{"file":out,"changedTracks":len(in.Tracks),"notice":"Import the NEW DAWproject into a new/backup Cubase project, listen and verify plugin mapping."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"vocal_backup_source",Description:"Make byte-identical SHA256-verified backup of an existing .svp/.vsqx/.ppsf (including ALL tuning, retakes, expression and project metadata). Never alters the original; call before proposing vocal edits."},func(ctx context.Context,req *mcp.CallToolRequest,in vocalSourceInput)(*mcp.CallToolResult,response,error){
  src:=pathFromProject(in.Source)
  ext:=strings.ToLower(filepath.Ext(src))
  if ext!=".svp"&&ext!=".vsqx"&&ext!=".ppsf"{return nil,response{},errors.New("supported vocal source formats: .svp, .vsqx, .ppsf")}
  f,err:=os.Open(src);if err!=nil{return nil,response{},err}
  defer f.Close()
  stat,err:=f.Stat();if err!=nil{return nil,response{},err}
  if !stat.Mode().IsRegular()||stat.Size()>1<<30{return nil,response{},errors.New("vocal source must be a regular file <= 1GiB")}
  root,err:=filepath.Abs(projectDir());if err!=nil{return nil,response{},err}
  folder:=filepath.Join(root,".cusic","vocal-backups")
  if err=os.MkdirAll(folder,0700);err!=nil{return nil,response{},err}
  tmp,err:=os.CreateTemp(folder,".cusic-vocal-*");if err!=nil{return nil,response{},err}
  defer os.Remove(tmp.Name())
  hash:=sha256.New()
  if _,err=io.Copy(io.MultiWriter(tmp,hash),f);err!=nil{tmp.Close();return nil,response{},err}
  if err=tmp.Close();err!=nil{return nil,response{},err}
  sum:=hex.EncodeToString(hash.Sum(nil))
  dst:=filepath.Join(folder,sum+ext)
  if _,err=os.Stat(dst);errors.Is(err,os.ErrNotExist){
   if err=os.Rename(tmp.Name(),dst);err!=nil{return nil,response{},err}
   if err=os.Chmod(dst,0444);err!=nil{return nil,response{},err}
  }else if err!=nil{return nil,response{},err}
  return output(map[string]any{"source":src,"backup":dst,"sha256":sum,"bytes":stat.Size(),"notice":"Source remains unchanged. Backup retains original full tuning and expression, unlike MIDI."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"vocal_vsqx_edit_existing",Description:"Non-destructive edit of existing Miku V4X note pitch/lyrics in a new VSQX: preserves original mCtrl pitch bends, expression controllers, notes, voicebank, nStyle and note timing. Fails if original pitches changed. NEVER overwrites source."},func(ctx context.Context,req *mcp.CallToolRequest,in vsqxEditsInput)(*mcp.CallToolResult,response,error){
  from:=pathFromProject(in.Source);dst,err:=outputPath(in.Out);if err!=nil{return nil,response{},err}
  b,err:=os.ReadFile(from);if err!=nil{return nil,response{},err}
  out,err:=vsqx.EditExisting(b,in.Changes);if err!=nil{return nil,response{},err}
  if err=os.MkdirAll(filepath.Dir(dst),0755);err!=nil{return nil,response{},err}
  if err=os.WriteFile(dst,out,0644);err!=nil{return nil,response{},err}
  return output(map[string]any{"source":from,"output":dst,"edits":len(in.Changes),"preservation":"existing note style and mCtrl automation retained; timeline untouched"})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"vocal_source_status" ,Description:"Discover original VSQX/SVP files (not flattened to MIDI). Existing pitch curves/expression are preserved by leaving their source files untouched."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  files:=[]string{}
  root:=projectDir()
  err:=filepath.WalkDir(root,func(p string,e os.DirEntry,err error)error{
   if err!=nil{return err}
   if e.IsDir(){if e.Name()==".cusic"||e.Name()==".git"{return filepath.SkipDir};return nil}
   ext:=strings.ToLower(filepath.Ext(p));if ext==".svp"||ext==".vsqx"||ext==".ppsf"{rel,_:=filepath.Rel(root,p);files=append(files,rel)}
   return nil
  })
  if err!=nil{return nil,response{},err}
  return output(map[string]any{"vocalSources":files,"policy":"Never replace or flatten a tuned source. For new vocals export additional parts. For existing expressive edits use the native editor and keep the original file."})
 })
 _ = audio.Stats{} // maintained dependency for sound-analysis API adapters
 return server
}
func Run()error{return NewServer().Run(context.Background(),&mcp.StdioTransport{})}
