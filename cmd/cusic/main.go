package main

import (
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "os"
 "io/fs"
 "os/exec"
 "path/filepath"
 "strconv"
 "strings"

 "github.com/ichiken26/cusic/internal/agent"
 "github.com/ichiken26/cusic/internal/midifile"
 "github.com/ichiken26/cusic/internal/dawproject"
 "github.com/ichiken26/cusic/internal/tuxguitar"
 "github.com/ichiken26/cusic/internal/vsqx"
 "github.com/ichiken26/cusic/internal/store"
)

func main(){
 if err:=run(os.Args[1:]);err!=nil{fmt.Fprintln(os.Stderr,"cusic:",err);os.Exit(1)}
}
func run(args []string)error{
 if len(args)==0{return help()}
 switch args[0]{
 case "help","--help","-h":return help()
 case "mcp":return agent.Run()
 case "setup":return setup(args[1:])
 case "init":
  f:=flag.NewFlagSet("init",flag.ContinueOnError)
  name:=f.String("name","Untitled","project name");tempo:=f.Int("tempo",120,"BPM");meter:=f.String("meter","4/4","time signature")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  directory:=".";if f.NArg()>0{directory=f.Arg(0)}
  var n,d int;if _,err:=fmt.Sscanf(*meter,"%d/%d",&n,&d);err!=nil{return err}
  if err:=store.Init(directory,store.Settings{Name:*name,Tempo:*tempo,MeterNum:n,MeterDen:d});err!=nil{return err}
  fmt.Printf("Initialized %s (%s)\n",directory,store.DBPath(directory));return nil
 }
 working:=".";if v:=os.Getenv("CUSIC_PROJECT");v!=""{working=v}
 s,err:=store.Open(working);if err!=nil{return err};defer s.Close()
 switch args[0]{
 case "status":
  settings,e:=s.Settings();if e!=nil{return e}
  tracks,e:=s.Tracks();if e!=nil{return e}
  rev,e:=s.Revisions(1);if e!=nil{return e}
  head:=int64(0);if len(rev)>0{head=rev[0].ID}
  return printJSON(struct{Project store.Settings `json:"project"`;Head int64 `json:"head"`;Tracks []store.Track `json:"tracks"`}{settings,head,tracks})
 case "track":return track(s,args[1:])
 case "note":return note(s,args[1:])
 case "inspect":return inspect(s,args[1:])
 case "patch":return patch(s,args[1:])
 case "log":
  f:=flag.NewFlagSet("log",flag.ContinueOnError);limit:=f.Int("n",20,"limit")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  rs,err:=s.Revisions(*limit);if err!=nil{return err}
  for _,r:=range rs{fmt.Printf("%d  %s  %s (%s)\n",r.ID,r.At,r.Message,r.Operation)}
  return nil
 case "diff":
  if len(args)<2{return errors.New("usage: cusic diff <revision>")}
  id,err:=strconv.ParseInt(args[1],10,64);if err!=nil{return err}
  r,err:=s.Revision(id);if err!=nil{return err};return printJSON(r)
 case "revert":
  if len(args)<2{return errors.New("usage: cusic revert <revision> (HEAD only)")}
  id,err:=strconv.ParseInt(args[1],10,64);if err!=nil{return err}
  rev,err:=s.Revert(id);if err!=nil{return err}
  fmt.Printf("Reverted revision %d (new HEAD %d)\n",id,rev);return nil
 case "import":return importSong(s,args[1:])
 case "suno":return suno(s,args[1:])
 case "audio":return audioCommand(s,args[1:])
 case "sync":return syncCommand(s,args[1:])
 case "export":return export(s,args[1:])
 case "validate":
  var result string
  if err:=s.DB.QueryRow("PRAGMA integrity_check").Scan(&result);err!=nil{return err}
  if result!="ok"{return fmt.Errorf("sqlite integrity_check: %s",result)}
  var violations int
  rs,err:=s.DB.Query("PRAGMA foreign_key_check");if err!=nil{return err}
  for rs.Next(){violations++};rs.Close()
  if violations>0{return fmt.Errorf("%d foreign key violations",violations)}
  fmt.Println("OK: SQLite integrity and foreign keys");return nil
 default:return fmt.Errorf("unknown command %q; run cusic help",args[0])
 }
}
func help()error{
 fmt.Print("cusic — local-first music agent CLI\n\n")
 fmt.Print("  cusic setup [PROJECT_DIR]  # One-time Claude/Codex MCP setup\n  cusic mcp                  # stdio MCP server\n")
 fmt.Print("  cusic init --name Demo --tempo 168 [DIR]\n")
 fmt.Print("  cusic status\n  cusic track add --name Bass [--kind instrument|drums|vocal] [--singer teto_sv2]\n")
 fmt.Print("  cusic track list\n  cusic note add --track Bass --tick 0 --duration 480 --pitch 40 --velocity 100\n")
 fmt.Print("  cusic inspect --track Bass [--bars 17:24]\n  cusic patch apply ./patch.json\n")
 fmt.Print("  cusic log [-n 20]\n  cusic diff REV\n  cusic revert REV  (HEAD only)\n")
 fmt.Print("  cusic import song.mid\n  cusic export --format midi --out song.mid\n")
 fmt.Print("  cusic export --format synthv --track Teto --bars 1:8 --out teto.json\n")
 fmt.Print("  cusic import demo.tg | demo.dawproject | demo.mid\n  cusic suno import ./stems\n  cusic suno separate --out ./stems song.wav\n  cusic suno transcribe --out ./midi stem.wav\n  cusic audio list\n")
 fmt.Print("  cusic export --format vsqx --track Miku --template piapro.vsqx --out miku.vsqx\n  cusic export --format dawproject --out cubase.dawproject\n")
 fmt.Print("  cusic sync plan --from cubase-export.dawproject\n  cusic sync pull --from cubase-export.dawproject --base-revision 12\n")
 fmt.Print("  cusic validate\n\nUse CUSIC_PROJECT=/path/to/song to target a different project.\n")
 return nil
}
func printJSON(value any)error{return json.NewEncoder(os.Stdout).Encode(value)}
func track(s *store.Store,args []string)error{
 if len(args)==0{return errors.New("usage: cusic track add|list")}
 switch args[0]{
 case "list":ts,err:=s.Tracks();if err!=nil{return err};return printJSON(ts)
 case "add":
  f:=flag.NewFlagSet("track add",flag.ContinueOnError)
  name:=f.String("name","","name");kind:=f.String("kind","instrument","kind");singer:=f.String("singer","","singer")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  t,r,err:=s.AddTrack(*name,*kind,*singer);if err!=nil{return err}
  fmt.Printf("Added %s (%s), revision %d\n",t.Name,t.ID,r);return nil
 }
 return errors.New("usage: cusic track add|list")
}
func note(s *store.Store,args []string)error{
 if len(args)==0||args[0]!="add"{return errors.New("usage: cusic note add --track NAME --tick N --duration N --pitch N --velocity N")}
 f:=flag.NewFlagSet("note add",flag.ContinueOnError)
 tr:=f.String("track","","track name or id");tick:=f.Int64("tick",0,"absolute MIDI tick (480 PPQ)")
 duration:=f.Int64("duration",480,"ticks");pitch:=f.Int("pitch",60,"MIDI key");velocity:=f.Int("velocity",100,"velocity 1..127");lyric:=f.String("lyric","","lyric")
 if err:=f.Parse(args[1:]);err!=nil{return err}
 n,r,err:=s.AddNote(*tr,store.InputNote{Tick:*tick,Duration:*duration,Pitch:*pitch,Velocity:*velocity,Lyric:*lyric})
 if err!=nil{return err};fmt.Printf("Added note %s, revision %d\n",n.ID,r);return nil
}
func parseBars(v string)(int,int,error){
 if v==""{return 0,0,nil}
 var a,b int
 if _,err:=fmt.Sscanf(v,"%d:%d",&a,&b);err!=nil{return 0,0,errors.New("bars must be START:END")}
 if a<1||b<a{return 0,0,errors.New("invalid inclusive bar range")};return a,b,nil
}
func noteRange(settings store.Settings,bars string)(int64,int64,error){
 from,to,err:=parseBars(bars);if err!=nil{return 0,0,err}
 if from==0{return 0,0,nil}
 ticks:=int64(settings.MeterNum)*store.PPQ*4/int64(settings.MeterDen)
 return int64(from-1)*ticks,int64(to)*ticks,nil
}
func inspect(s *store.Store,args []string)error{
 f:=flag.NewFlagSet("inspect",flag.ContinueOnError);tr:=f.String("track","","track name");bars:=f.String("bars","","inclusive bars a:b")
 if err:=f.Parse(args);err!=nil{return err}
 settings,err:=s.Settings();if err!=nil{return err}
 if *tr==""{return printJSON(settings)}
 t,err:=s.Track(*tr);if err!=nil{return err}
 from,to,err:=noteRange(settings,*bars);if err!=nil{return err}
 ns,err:=s.Notes(t.ID,from,to);if err!=nil{return err}
 return printJSON(struct{Track store.Track `json:"track"`;Notes []store.Note `json:"notes"`}{t,ns})
}
func patch(s *store.Store,args []string)error{
 if len(args)<2||args[0]!="apply"{return errors.New("usage: cusic patch apply FILE.json")}
 b,err:=os.ReadFile(args[1]);if err!=nil{return err}
 var p store.Patch
 dec:=json.NewDecoder(strings.NewReader(string(b)));dec.DisallowUnknownFields()
 if err=dec.Decode(&p);err!=nil{return err}
 if p.Notes==nil{return errors.New("patch.notes is required (use [] to delete notes)")}
 r,err:=s.ApplyPatch(p);if err!=nil{return err};fmt.Printf("Applied patch; HEAD %d\n",r);return nil
}
func importSong(s *store.Store,args []string)error{
 if len(args)<1{return errors.New("usage: cusic import song.mid|song.tg|session.dawproject")}
 filename:=args[0];ext:=strings.ToLower(filepath.Ext(filename))
 var song midifile.Song
 var err error
 var importedAudio []dawproject.ImportedAudio
 switch ext {
 case ".mid",".midi":song,err=midifile.ReadFile(filename)
 case ".tg":song,err=tuxguitar.ReadFile(filename)
 case ".dawproject":
  var data dawproject.Imported;data,err=dawproject.ReadFile(filename)
  song=data.Song;importedAudio=data.Audio
 default:return fmt.Errorf("unsupported import format %q",ext)
 }
 if err!=nil{return err}
 if len(song.Tracks)>0{
  rev,err:=s.AddImported(song.Tracks,song.Tempo,song.MeterNum,song.MeterDen)
  if err!=nil{return err}
  fmt.Printf("Imported %d note tracks; HEAD %d\n",len(song.Tracks),rev)
 }
 // Embedded audio is extracted into a temporary file then hashed and copied
 // to the local project. Never trust arbitrary ZIP paths.
 for _,source:=range importedAudio{
  directory,e:=os.MkdirTemp("","cusic-audio-*");if e!=nil{return e}
  file:=filepath.Join(directory,"stem.wav")
  e=dawproject.ExtractAudio(filename,source.ArchivePath,file)
  if e==nil{_,_,e=s.AddAudioFile(file,source.Name)}
  os.RemoveAll(directory)
  if e!=nil{return e}
 }
 if len(song.Tracks)==0&&len(importedAudio)==0{return errors.New("no importable notes or WAV stems")}
 fmt.Println("Note: non-note MIDI expression, original VST state and tablature technique are not preserved.")
 return nil
}
func suno(s *store.Store,args []string)error{
 if len(args)<1{return errors.New("usage: cusic suno import STEMS_DIR | separate --out DIR MIX.wav | transcribe --out DIR STEM.wav")}
 switch args[0]{
 case "import":
  if len(args)<2{return errors.New("usage: cusic suno import STEMS_DIR")}
  folder:=args[1]
  found:=0
  err:=filepath.WalkDir(folder,func(filename string,entry fs.DirEntry,visitErr error)error{
   if visitErr!=nil{return visitErr}
   if entry.IsDir()||strings.ToLower(filepath.Ext(entry.Name()))!=".wav"{return nil}
   if found>=2048{return errors.New("too many WAV stems (max 2048)")}
   rel,err:=filepath.Rel(folder,filename);if err!=nil{return err}
   name:=strings.TrimSuffix(filepath.ToSlash(rel),filepath.Ext(rel))
   a,rev,err:=s.AddAudioFile(filename,name);if err!=nil{return err}
   fmt.Printf("Imported %s (%.2fs, %.0f Hz), HEAD %d\n",a.Name,a.Stats.Duration,float64(a.Stats.SampleRate),rev);found++
   return nil
  })
  if err!=nil{return err}
  if found==0{return errors.New("no WAV stems under folder; export WAVs from Suno Studio first")}
  return nil
 case "separate","transcribe":
  f:=flag.NewFlagSet("suno "+args[0],flag.ContinueOnError)
  out:=f.String("out","","new directory for generated stems/MIDI")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  if *out==""||f.NArg()!=1{return fmt.Errorf("usage: cusic suno %s --out NEW_DIR source.wav",args[0])}
  source:=f.Arg(0)
  if _,err:=os.Stat(source);err!=nil{return err}
  if _,err:=os.Stat(*out);err==nil{return errors.New("output directory exists: choose a new directory to avoid overwriting audio")}else if !errors.Is(err,os.ErrNotExist){return err}
  if err:=os.MkdirAll(*out,0755);err!=nil{return err}
  var tool string;var argv []string
  if args[0]=="separate"{
   tool="demucs";argv=[]string{"-o",*out,source}
  }else{
   tool="basic-pitch";argv=[]string{*out,source}
  }
  if _,err:=exec.LookPath(tool);err!=nil{return fmt.Errorf("%s is not installed in PATH; see README for optional audio dependencies",tool)}
  cmd:=exec.Command(tool,argv...)
  cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr
  if err:=cmd.Run();err!=nil{return fmt.Errorf("%s failed: %w",tool,err)}
  fmt.Printf("Generated %s files in %s; inspect results before importing\n",args[0],*out)
  return nil
 }
 return errors.New("usage: cusic suno import|separate|transcribe")
}
func audioCommand(s *store.Store,args []string)error{
 if len(args)!=1||args[0]!="list"{return errors.New("usage: cusic audio list")}
 a,err:=s.Assets();if err!=nil{return err};return printJSON(a)
}
func syncCommand(s *store.Store,args []string)error{
 if len(args)<1{return errors.New("usage: cusic sync plan|pull --from cubase.dawproject [--base-revision REV]")}
 action:=args[0];if action!="plan"&&action!="pull"{return errors.New("sync supports plan or pull")}
 f:=flag.NewFlagSet("sync "+action,flag.ContinueOnError)
 filename:=f.String("from","","Cubase-exported .dawproject")
 base:=f.Int64("base-revision",-1,"expected cusic HEAD; mandatory for pull")
 if err:=f.Parse(args[1:]);err!=nil{return err}
 if *filename==""{return errors.New("--from required")}
 parsed,err:=dawproject.ReadFile(*filename);if err!=nil{return err}
 cfg,err:=s.Settings();if err!=nil{return err}
 if cfg.Tempo!=parsed.Song.Tempo||cfg.MeterNum!=parsed.Song.MeterNum||cfg.MeterDen!=parsed.Song.MeterDen{return errors.New("Cubase tempo/meter differs from cusic; refusing sync")}
 plan,err:=s.PreviewSync(parsed.Song.Tracks);if err!=nil{return err}
 if action=="plan"{return printJSON(plan)}
 if *base<0{return errors.New("--base-revision required; run cusic sync plan first")}
 rev,err:=s.SyncTracks(parsed.Song.Tracks,*base);if err!=nil{return err}
 fmt.Printf("Pulled Cubase note edits; HEAD %d\n",rev)
 fmt.Println("Audio tracks and VST settings are not modified by sync pull.")
 return nil
}
func writeFileAtomic(path string,b []byte)error{
 if _,err:=os.Stat(path);err==nil{return fmt.Errorf("output already exists: %s; choose a new path",path)}else if !errors.Is(err,os.ErrNotExist){return err}
 dir:=filepath.Dir(path);if err:=os.MkdirAll(dir,0755);err!=nil{return err}
 tmp,err:=os.CreateTemp(dir,".cusic-out-*");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 if _,err=tmp.Write(b);err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(tmp.Name(),path)
}
func export(s *store.Store,args []string)error{
 f:=flag.NewFlagSet("export",flag.ContinueOnError)
 format:=f.String("format","midi","midi, synthv, vsqx or dawproject");out:=f.String("out","","output file")
 template:=f.String("template","","Piapro V4X template VSQX")
 track:=f.String("track","","vocal track");bars:=f.String("bars","","inclusive bar range for synthv")
 if err:=f.Parse(args);err!=nil{return err}
 if *out==""{return errors.New("--out required")}
 settings,err:=s.Settings();if err!=nil{return err}
 switch *format{
 case "midi":
  ts,err:=s.Tracks();if err!=nil{return err}
  ns:=make(map[string][]store.Note)
  for _,t:=range ts{notes,e:=s.Notes(t.ID,0,0);if e!=nil{return e};ns[t.ID]=notes}
  b,e:=midifile.Write(settings,ts,ns);if e!=nil{return e}
  if err=writeFileAtomic(*out,b);err!=nil{return err}
 case "dawproject":
  tracks,err:=s.Tracks();if err!=nil{return err}
  notes:=make(map[string][]store.Note)
  for _,t:=range tracks{items,e:=s.Notes(t.ID,0,0);if e!=nil{return e};notes[t.ID]=items}
  assets,err:=s.Assets();if err!=nil{return err}
  audioFiles:=[]dawproject.AudioFile{}
  for _,a:=range assets{
   abs,e:=s.AssetAbsolute(a);if e!=nil{return e}
   audioFiles=append(audioFiles,dawproject.AudioFile{Name:a.Name,AbsolutePath:abs,SampleRate:a.Stats.SampleRate,Channels:a.Stats.Channels,Duration:a.Stats.Duration})
  }
  if err=dawproject.WriteFile(*out,settings,tracks,notes,audioFiles);err!=nil{return err}
 case "vsqx":
  if *track==""||*template==""{return errors.New("vsqx requires --track and --template from Piapro Studio V4X")}
  t,err:=s.Track(*track);if err!=nil{return err}
  if t.Kind!="vocal"{return errors.New("vsqx requires a vocal track")}
  from,to,err:=noteRange(settings,*bars);if err!=nil{return err}
  notes,err:=s.Notes(t.ID,from,to);if err!=nil{return err}
  if from!=0{for _,n:=range notes{if n.Tick<from||n.Tick+n.Duration>to{return errors.New("VSQX note crosses selection boundary")}}}
  b,err:=os.ReadFile(*template);if err!=nil{return err}
  encoded,err:=vsqx.Export(b,settings,t,notes);if err!=nil{return err}
  if err=writeFileAtomic(*out,encoded);err!=nil{return err}
  fmt.Fprintln(os.Stderr,"Note: review Japanese phonemes and vocal tuning inside Piapro Studio.")
 case "synthv":
  if *track==""{return errors.New("--track required for synthv")}
  t,err:=s.Track(*track);if err!=nil{return err}
  if t.Kind!="vocal"{return errors.New("SynthV export requires a vocal track")}
  from,to,err:=noteRange(settings,*bars);if err!=nil{return err}
  ns,err:=s.Notes(t.ID,from,to);if err!=nil{return err}
  if to==0{
   for _,n:=range ns{if n.Tick+n.Duration>to{to=n.Tick+n.Duration}}
   if to==0{return errors.New("cannot export empty vocal track without --bars")}
  }
  for _,n:=range ns{if n.Tick<from||n.Tick+n.Duration>to{return errors.New("a note crosses the synthv export boundary; use a wider bar range")}}
  payload:=struct{
   Version int `json:"version"`
   TrackID string `json:"trackId"`
   TrackName string `json:"trackName"`
   PPQ int64 `json:"ppq"`
   FromTick int64 `json:"fromTick"`
   ToTick int64 `json:"toTick"`
   Notes []store.Note `json:"notes"`
  }{1,t.ID,t.Name,store.PPQ,from,to,ns}
  b,err:=json.MarshalIndent(payload,"","  ");if err!=nil{return err}
  if err=writeFileAtomic(*out,append(b,'\n'));err!=nil{return err}
 default:return errors.New("unknown format; use midi or synthv")
 }
 fmt.Printf("Exported %s to %s\n",*format,*out);return nil
}
