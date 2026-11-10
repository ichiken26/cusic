package main

import (
 "encoding/json"
 "errors"
 "flag"
 "fmt"
 "os"

 "github.com/ichiken26/cusic/internal/store"
 "github.com/ichiken26/cusic/internal/vsqx"
)

func vocalCommand(s *store.Store,args []string) error {
 if len(args)==0{return errors.New("usage: cusic vocal voices | vocal apply --file vocal-edits.json")}
 switch args[0]{
 case "voices":
  if len(args)!=1{return errors.New("usage: cusic vocal voices")}
  voices,err:=vsqx.ScanVoices(s.Root)
  if err!=nil{return err}
  return printJSON(map[string]any{"voices":voices,"hint":"These are real voice references from VSQX files in this project; use --voice Original, Solid, etc when exporting. If Original is missing, export with a known voice and change it in Piapro."})
 case "apply":
  f:=flag.NewFlagSet("vocal apply",flag.ContinueOnError)
  file:=f.String("file","","JSON changes file (non-destructive note IDs)")
  if err:=f.Parse(args[1:]);err!=nil{return err}
  if *file==""||f.NArg()!=0{return errors.New("usage: cusic vocal apply --file vocal-edits.json")}
  data,err:=os.ReadFile(*file);if err!=nil{return err}
  var p store.VocalPatch
  if err=json.Unmarshal(data,&p);err!=nil{return err}
  result,err:=s.ApplyVocalPatch(p);if err!=nil{return err}
  fmt.Printf("Vocal notes updated on %s: +%d, modified %d, removed %d; HEAD %d\n",result.Track.Name,result.Added,result.Updated,result.Deleted,result.Revision)
  return nil
 default:
  return fmt.Errorf("unsupported vocal subcommand %q",args[0])
 }
}
