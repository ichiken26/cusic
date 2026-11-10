package main

import(
 "errors"
 "flag"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 "github.com/ichiken26/cusic/internal/store"
 "github.com/ichiken26/cusic/internal/svp"
)

// svpCommand directly writes an edited SynthV project, not a MIDI/JS bridge.
// Source file is never modified; its original vocal track remains as a muted
// copy in the NEW project, with a new voice/retake-preserving clone.
func svpCommand(s *store.Store,args []string)error{
 if len(args)==0{return errors.New("usage: cusic svp plan|apply --from ORIGINAL.svp --track Teto [--out NEW.svp]")}
 action:=args[0]
 if action!="plan"&&action!="apply"{return errors.New("usage: cusic svp plan|apply")}
 f:=flag.NewFlagSet("svp "+action,flag.ContinueOnError)
 source:=f.String("from","","existing Synthesizer V .svp file")
 track:=f.String("track","Teto","cusic vocal track with proposed notes")
 originalTrack:=f.String("source-track","","original SVP source track to clone (defaults to --track)")
 payloadFile:=f.String("payload","","optional pre-exported cusic synthv JSON (otherwise use cusic track directly)")
 out:=f.String("out","","new SVP path (apply)")
 if err:=f.Parse(args[1:]);err!=nil{return err}
 if *source==""||f.NArg()!=0{return errors.New("svp requires --from ORIGINAL.svp")}
 if strings.ToLower(filepath.Ext(*source))!=".svp"{return errors.New("SVP input must end in .svp")}
 name:=*originalTrack
 if name==""{name=*track}
 if name==""{return errors.New("--source-track or --track required")}
 original,err:=os.ReadFile(*source);if err!=nil{return err}
 var payload svp.Payload
 if *payloadFile!=""{
  data,e:=os.ReadFile(*payloadFile);if e!=nil{return e}
  payload,e=svp.LoadPayload(data);if e!=nil{return e}
 }else{
  payload,err=svp.PayloadFromStore(s,original,name,*track)
  if err!=nil{return err}
 }
 created,summary,err:=svp.Apply(original,payload,name);if err!=nil{return err}
 if action=="plan"{
  return printJSON(map[string]any{"safeToApply":true,"preview":summary,"source":*source,"writeOccurred":false})
 }
 if *out==""||strings.ToLower(filepath.Ext(*out))!=".svp"{return errors.New("svp apply requires --out NEW.svp")}
 if err=writeFileAtomic(*out,created);err!=nil{return err}
 fmt.Printf("Created %s; source %q muted, new track %q has %d edited notes. Original file untouched.\n",*out,summary.SourceTrack,summary.NewTrack,summary.AddedNotes)
 fmt.Println("The vocal pronunciation report below is a heuristic; review phonemes and retakes inside SynthV.")
 return printJSON(summary)
}
