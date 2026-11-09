package main

import(
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "github.com/ichiken26/cusic/internal/dawproject"
 "github.com/ichiken26/cusic/internal/store"
)

// setup performs the one-time integration. After this, agents can work solely
// through project-local MCP without knowing cusic CLI commands.
func setup(args []string)error{
 if len(args)>1{return errors.New("usage: cusic setup [PROJECT_DIR]")}
 root:=".";if len(args)==1{root=args[0]}
 absolute,err:=filepath.Abs(root);if err!=nil{return err}
 if err:=os.MkdirAll(absolute,0755);err!=nil{return err}
 exe,err:=os.Executable();if err!=nil{return err}
 exe,err=filepath.Abs(exe);if err!=nil{return err}
 if _,err=os.Stat(store.DBPath(absolute));errors.Is(err,os.ErrNotExist){
  entries,e:=os.ReadDir(absolute);if e!=nil{return e}
  candidates:=[]string{}
  for _,entry:=range entries{
   if !entry.IsDir()&&strings.EqualFold(filepath.Ext(entry.Name()),".dawproject"){candidates=append(candidates,entry.Name())}
  }
  if len(candidates)==1{
   filename:=filepath.Join(absolute,candidates[0]);parsed,e:=dawproject.ReadFile(filename);if e!=nil{return e}
   cfg:=store.Settings{Name:strings.TrimSuffix(candidates[0],filepath.Ext(candidates[0])),Tempo:parsed.Song.Tempo,MeterNum:parsed.Song.MeterNum,MeterDen:parsed.Song.MeterDen}
   if e=store.Init(absolute,cfg);e!=nil{return e}
   s,e:=store.Open(absolute);if e!=nil{return e}
   if len(parsed.Song.Tracks)>0{
    _,e=s.AddImported(parsed.Song.Tracks,parsed.Song.Tempo,parsed.Song.MeterNum,parsed.Song.MeterDen)
   }
   if e==nil{
    for _,a:=range parsed.Audio{
     tmp,tmpErr:=os.MkdirTemp("","cusic-setup-audio-*");if tmpErr!=nil{e=tmpErr;break}
     extracted:=filepath.Join(tmp,"stem.wav")
     tmpErr=dawproject.ExtractAudio(filename,a.ArchivePath,extracted)
     if tmpErr==nil{_,_,tmpErr=s.AddAudioFile(extracted,a.Name)}
     os.RemoveAll(tmp)
     if tmpErr!=nil{e=tmpErr;break}
    }
   }
   s.Close();if e!=nil{return e}
   fmt.Println("Initialized from "+candidates[0]+" (note tracks and embedded WAV imported)")
  }else{
   fmt.Fprintln(os.Stderr,"No unique DAWproject found. MCP can initialize an audio-first project with pending tempo, import Suno stems, then propose BPM from the drums.")
  }
 }else if err!=nil{return err}
 // Claude Code project MCP. Merge existing config, do not clobber other servers.
 claudePath:=filepath.Join(absolute,".mcp.json")
 config:=map[string]any{}
 if b,e:=os.ReadFile(claudePath);e==nil{
  if e=json.Unmarshal(b,&config);e!=nil{return fmt.Errorf("existing .mcp.json invalid: %w",e)}
 }else if !errors.Is(e,os.ErrNotExist){return e}
 servers:=map[string]any{}
 if prev,ok:=config["mcpServers"];ok{
  typed,ok:=prev.(map[string]any);if !ok{return errors.New("unexpected mcpServers in existing .mcp.json")}
  servers=typed
 }
 if _,exists:=servers["cusic"];exists{
  fmt.Fprintln(os.Stderr,"Existing Claude cusic MCP entry kept; verify its project path.")
 }else{
  servers["cusic"]=map[string]any{"type":"stdio","command":exe,"args":[]string{"mcp"},"env":map[string]string{"CUSIC_PROJECT":absolute}}
  config["mcpServers"]=servers
  b,e:=json.MarshalIndent(config,"","  ");if e!=nil{return e}
  if e=os.WriteFile(claudePath,append(b,'\n'),0644);e!=nil{return e}
 }
 // Codex project scoped config is loaded only after the user trusts the project.
 codexPath:=filepath.Join(absolute,".codex","config.toml")
 existing,e:=os.ReadFile(codexPath);if e!=nil&&!errors.Is(e,os.ErrNotExist){return e}
 if !strings.Contains(string(existing),"[mcp_servers.cusic]"){
  if e=os.MkdirAll(filepath.Dir(codexPath),0755);e!=nil{return e}
  updated:=string(existing)
  if updated!=""&&!strings.HasSuffix(updated,"\n"){updated+="\n"}
  updated+="\n[mcp_servers.cusic]\n"
  updated+="command = "+strconv.Quote(exe)+"\n"
  updated+="args = [\"mcp\"]\n"
  updated+="cwd = "+strconv.Quote(absolute)+"\n"
  updated+="env = { CUSIC_PROJECT = "+strconv.Quote(absolute)+" }\n"
  if e=os.WriteFile(codexPath,[]byte(updated),0644);e!=nil{return e}
 }
 // Project-local instructions. Do not overwrite user AGENTS.md or CLAUDE.md.
 workflow:=`# cusic workflow

This folder is an existing song. You are the user's music-production collaborator.
The user gives free-form instructions and does NOT need to learn cusic commands.

1. Discover the local cusic MCP tools and call project_inventory / project_status.
2. If only .cpr exists, ask the user for ONE Cubase export to .dawproject.
   Do not pretend you can extract audio/MIDI from proprietary .cpr automatically.
3. Import the DAWproject through project_import. Inspect audio assets and track notes.
4. For arrangement edits, create bounded track patches based on HEAD.
   Keep existing live guitar/bass recordings untouched; new MIDI is an arrangement suggestion.
5. Before mixing, prefer mix_analyze_spectral (NumPy/SciPy FFT) for\n   spectrum overlap, spectral centroid and stereo correlation. If Python is not\n   configured, fall back to mix_analyze_stems for basic Go-only WAV statistics.\n   Report proposed EQ/comp/pan
   settings, distinguishing measured findings from creative recommendations.
   cusic cannot directly change the running Cubase app. Use mix_plan then mix_apply\n   to create a NEW DAWproject with generic EQ, fader and pan values when approved.
6. NEVER delete or overwrite any existing .svp, .vsqx, .ppsf or .cpr file.
   Vocal expression, pitch automation and retakes are the artist's original work.
   Use vocal_backup_source before any targeted edit. For Miku, vocal_vsqx_edit_existing\n   changes note pitch/lyrics in a NEW VSQX without discarding mCtrl or nStyle.\n   For Teto, prefer the TS/Zod safe existing-note script over the destructive\n   note-replacement bridge. Avoid changing onset/duration of tuned notes.\n   Do not flatten tuned vocals to MIDI and call it lossless. Produce separate
   accompaniment/harmony alternatives and seek approval for vocal modifications.
7. Avoid overwriting any file. Work in outputs with versioned filenames.
8. Show the user a clear summary of proposed changes rather than CLI syntax.
`
 guide:=filepath.Join(absolute,"CUSIC_WORKFLOW.md")
 if _,e:=os.Stat(guide);errors.Is(e,os.ErrNotExist){if e=os.WriteFile(guide,[]byte(workflow),0644);e!=nil{return e}}
 agents:=filepath.Join(absolute,"AGENTS.md")
 if _,e:=os.Stat(agents);errors.Is(e,os.ErrNotExist){
  _=os.WriteFile(agents,[]byte("# Agent instructions\n\nRead CUSIC_WORKFLOW.md before making changes to this music project.\n"),0644)
 }else{fmt.Fprintln(os.Stderr,"AGENTS.md exists: add a reference to CUSIC_WORKFLOW.md for full agent instructions")}
 claude:=filepath.Join(absolute,"CLAUDE.md")
 if _,e:=os.Stat(claude);errors.Is(e,os.ErrNotExist){
  _=os.WriteFile(claude,[]byte("# Claude Code instructions\n\nRead CUSIC_WORKFLOW.md and use the cusic MCP tools.\n"),0644)
 }else{fmt.Fprintln(os.Stderr,"CLAUDE.md exists: add a reference to CUSIC_WORKFLOW.md for full agent instructions")}
 fmt.Println("cusic setup complete in "+absolute)
 fmt.Println("Open this folder in Claude Code or Codex and ask in plain language. Trust/enable the local MCP server when prompted.")
 return nil
}
