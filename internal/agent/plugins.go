package agent

import (
 "context"
 "errors"
 "path/filepath"

 "github.com/ichiken26/cusic/internal/dawproject"
 "github.com/ichiken26/cusic/internal/mix"
 "github.com/ichiken26/cusic/internal/store"
 "github.com/modelcontextprotocol/go-sdk/mcp"
)

type deviceInspectInput struct{ Source string `json:"source"` }
type devicePlanInput struct{
 Source string `json:"source"`
 Out string `json:"out,omitempty"`
 Operations []mix.DeviceOperation `json:"operations"`
 Confirm bool `json:"confirm,omitempty"`
}
type sourceAttachInput struct{ File string `json:"file"`; Confirm bool `json:"confirm"` }

func registerPluginTools(server *mcp.Server){
 mcp.AddTool(server,&mcp.Tool{Name:"effects_inspect",Description:"Read DAWproject audio channels, insert effects, VST3 state paths and parameter metadata without altering anything."},func(ctx context.Context,req *mcp.CallToolRequest,in deviceInspectInput)(*mcp.CallToolResult,response,error){
  if in.Source==""{return nil,response{},errors.New("source DAWproject required")}
  report,err:=mix.InspectDevices(pathFromProject(in.Source));if err!=nil{return nil,response{},err}
  return output(report)
 })
 mcp.AddTool(server,&mcp.Tool{Name:"kontakt_diagnose",Description:"Inspect a DAWproject for Kontakt instrument tracks, external channels, panning, missing state references and routing warnings. Cannot decode Kontakt proprietary multi-output configuration."},func(ctx context.Context,req *mcp.CallToolRequest,in deviceInspectInput)(*mcp.CallToolResult,response,error){
  if in.Source==""{return nil,response{},errors.New("source DAWproject required")}
  report,err:=mix.InspectDevices(pathFromProject(in.Source));if err!=nil{return nil,response{},err}
  return output(map[string]any{"kontakt":report.Kontakt,"warnings":report.Warnings,"limitations":report.Limitations})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"effects_plan",Description:"Preview DAWproject generic compressor insert/update, cloned VST3 effects, supported XML parameter changes or new native state provided by caller. Opaque VST3 plugin parameters cannot be synthesized from knob values."},func(ctx context.Context,req *mcp.CallToolRequest,in devicePlanInput)(*mcp.CallToolResult,response,error){
  if in.Source==""{return nil,response{},errors.New("source DAWproject required")}
  if err:=mix.CheckDevices(pathFromProject(in.Source),mix.DevicePlan{Operations:in.Operations});err!=nil{return nil,response{},err}
  return output(map[string]any{"valid":true,"operations":in.Operations,"requiresApproval":true,"note":"This is a preview; no file has been modified. effects_apply requires explicit approval."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"effects_apply",Description:"Create a NEW DAWproject copy with approved effect changes, preserving existing VST3 state and media. Never edits .cpr; explicit confirm=true required."},func(ctx context.Context,req *mcp.CallToolRequest,in devicePlanInput)(*mcp.CallToolResult,response,error){
  if !in.Confirm{return nil,response{},errors.New("effects_apply requires explicit confirm=true")}
  out,err:=outputPath(in.Out);if err!=nil{return nil,response{},err}
  if in.Source==""{return nil,response{},errors.New("source DAWproject required")}
  if filepath.Ext(out)!=".dawproject"{return nil,response{},errors.New("output must be .dawproject")}
  if err=mix.ApplyDevices(pathFromProject(in.Source),out,mix.DevicePlan{Operations:in.Operations});err!=nil{return nil,response{},err}
  return output(map[string]any{"path":out,"operationsApplied":len(in.Operations),"note":"Import the NEW DAWproject to a backup Cubase project and confirm mapping. Unknown VST3 vendor state cannot be inferred."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"project_source_status",Description:"Show whether the original DAWproject, including instrument and opaque VST states, is retained for safe exports."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   src,err:=s.SourceDAWProject();if err!=nil{return nil,err}
   if src==nil{return map[string]any{"preserved":false},nil}
   file,err:=s.SourceArchive(src);if err!=nil{return nil,err}
   return map[string]any{"preserved":true,"archive":file,"sha256":src.SHA256},nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{Name:"project_source_attach",Description:"For a cusic project imported before source preservation, attach its ORIGINAL DAWproject only after verifying tempo/meter and matching note content. Requires confirm=true."},func(ctx context.Context,req *mcp.CallToolRequest,in sourceAttachInput)(*mcp.CallToolResult,response,error){
  if !in.Confirm{return nil,response{},errors.New("project_source_attach requires confirm=true")}
  if in.File==""{return nil,response{},errors.New("source DAWproject filename required")}
  return withStore(func(s *store.Store)(any,error){
   file:=pathFromProject(in.File)
   imported,err:=dawproject.ReadFile(file);if err!=nil{return nil,err}
   settings,err:=s.Settings();if err!=nil{return nil,err}
   if settings.Tempo!=imported.Song.Tempo||settings.MeterNum!=imported.Song.MeterNum||settings.MeterDen!=imported.Song.MeterDen{
    return nil,errors.New("tempo/meter mismatch, refusing unrelated original DAWproject")
   }
   if err=s.VerifySourceNotes(imported.Song.Tracks);err!=nil{return nil,err}
   names:=make([]string,0,len(imported.Song.Tracks))
   for _,tr:=range imported.Song.Tracks{names=append(names,tr.Name)}
   if err=s.SaveSourceDAWProject(file,names);err!=nil{return nil,err}
   return map[string]any{"preserved":true,"source":file,"noteTracks":len(names)},nil
  })
 })
}
