package main

import (
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 "github.com/ichiken26/cusic/internal/dawproject"
 "github.com/ichiken26/cusic/internal/mix"
 "github.com/ichiken26/cusic/internal/store"
)

func effectsCommand(args []string)error{
 if len(args)==0{return errors.New("usage: cusic effects inspect|plan|apply --from session.dawproject")}
 action:=args[0]
 if action!="inspect"&&action!="plan"&&action!="apply"{return errors.New("usage: cusic effects inspect|plan|apply")}
 flags:=flag.NewFlagSet("effects "+action,flag.ContinueOnError)
 from:=flags.String("from","","Cubase-exported source DAWproject")
 out:=flags.String("out","","new output DAWproject (apply)")
 planFile:=flags.String("plan","","JSON device operations file")
 confirm:=flags.Bool("confirm",false,"explicitly approve non-destructive output")
 if err:=flags.Parse(args[1:]);err!=nil{return err}
 if flags.NArg()!=0{return errors.New("unexpected effects arguments")}
 if *from==""{return errors.New("--from SOURCE.dawproject is required")}
 if strings.ToLower(filepath.Ext(*from))!=".dawproject"{return errors.New("effects commands require a .dawproject file (not .cpr)")}
 if action=="inspect"{
  report,err:=mix.InspectDevices(*from);if err!=nil{return err}
  return printJSON(report)
 }
 if *planFile==""{return errors.New("--plan operations.json is required")}
 body,err:=os.ReadFile(*planFile);if err!=nil{return err}
 var plan mix.DevicePlan
 dec:=json.NewDecoder(strings.NewReader(string(body)))
 dec.DisallowUnknownFields()
 if err=dec.Decode(&plan);err!=nil{return fmt.Errorf("invalid device plan: %w",err)}
 if err=mix.CheckDevices(*from,plan);err!=nil{return err}
 if action=="plan"{return printJSON(map[string]any{"valid":true,"operations":plan.Operations,"source":*from,"note":"No files have been changed. A vendor VST3 state cannot be synthesized from knob values; clone or replace with a known-good state."})}
 if !*confirm{return errors.New("effects apply requires --confirm after reviewing effects plan")}
 if *out==""||strings.ToLower(filepath.Ext(*out))!=".dawproject"{return errors.New("--out NEW_FILE.dawproject is required")}
 if err=mix.ApplyDevices(*from,*out,plan);err!=nil{return err}
 fmt.Printf("Created %s with %d device operations (source unchanged).\n",*out,len(plan.Operations))
 return nil
}
func sourceCommand(s *store.Store,args []string)error{
 if len(args)==0{return errors.New("usage: cusic source status|attach --from original.dawproject")}
 switch args[0]{
 case "status":
  snapshot,err:=s.SourceDAWProject();if err!=nil{return err}
  if snapshot==nil{return printJSON(map[string]any{"preserved":false,"hint":"Import a DAWproject or run source attach --from ORIGINAL.dawproject to preserve effects."})}
  file,err:=s.SourceArchive(snapshot)
  if err!=nil{return err}
  return printJSON(map[string]any{"preserved":true,"archive":file,"sha256":snapshot.SHA256,"importedTracks":len(snapshot.ImportedNoteHashes)})
 case "attach":
  f:=flag.NewFlagSet("source attach",flag.ContinueOnError)
  from:=f.String("from","","previously imported original DAWproject")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  if *from==""||f.NArg()>0{return errors.New("usage: cusic source attach --from original.dawproject")}
  src,err:=dawproject.ReadFile(*from);if err!=nil{return err}
  if err=s.VerifySourceNotes(src.Song.Tracks);err!=nil{return err}
  names:=make([]string,0,len(src.Song.Tracks))
  for _,tr:=range src.Song.Tracks{names=append(names,tr.Name)}
  if err=s.SaveSourceDAWProject(*from,names);err!=nil{return err}
  fmt.Printf("Saved original DAWproject effects/instrument state for future exports (%d note tracks).\n",len(names))
  return nil
 default:return errors.New("usage: cusic source status|attach --from original.dawproject")
 }
}
