package agent

import (
 "context"
 "errors"

 "github.com/ichiken26/cusic/internal/store"
 "github.com/ichiken26/cusic/internal/vsqx"
 "github.com/modelcontextprotocol/go-sdk/mcp"
)

type vocalApplyInput struct {
 Track string `json:"track"`
 Singer string `json:"singer,omitempty"`
 BaseRevision int64 `json:"baseRevision"`
 Message string `json:"message,omitempty"`
 Changes []store.VocalChange `json:"changes"`
 CreateTrack bool `json:"createTrack,omitempty"`
 ConfirmDelete bool `json:"confirmDelete,omitempty"`
}
func registerVocalTools(server *mcp.Server){
 mcp.AddTool(server,&mcp.Tool{Name:"vocal_voices",Description:"Discover voicebank IDs and names from genuine VSQX files inside this song project. No Piapro template selection or fake Original voice ID needed. If Original is not present, an available voice may be used with a clear warning, then adjusted in Piapro."},func(ctx context.Context,req *mcp.CallToolRequest,in empty)(*mcp.CallToolResult,response,error){
  voices,err:=vsqx.ScanVoices(projectDir());if err!=nil{return nil,response{},err}
  return output(map[string]any{"voices":voices,"explanation":"Use the voice field of score_export. Genuine installed bank metadata only; the tool does not guess unavailable Original IDs."})
 })
 mcp.AddTool(server,&mcp.Tool{Name:"vocal_notes_apply",Description:"Safely create missing vocal track and/or add/edit exact note IDs in one atomic revision. Does NOT delete/recreate whole bars, does NOT alter original VSQX or tuned vocal effects. Use after project_status/track_inspect; confirmDelete=true only for explicit note deletions."},func(ctx context.Context,req *mcp.CallToolRequest,in vocalApplyInput)(*mcp.CallToolResult,response,error){
  for _,change:=range in.Changes{
   if change.Action=="delete"&&!in.ConfirmDelete{return nil,response{},errors.New("vocal note deletion requires explicit confirmDelete=true")}
  }
  return withStore(func(s *store.Store)(any,error){
   return s.ApplyVocalPatch(store.VocalPatch{Track:in.Track,Singer:in.Singer,BaseRevision:in.BaseRevision,Message:in.Message,Changes:in.Changes,CreateTrack:in.CreateTrack})
  })
 })
}
