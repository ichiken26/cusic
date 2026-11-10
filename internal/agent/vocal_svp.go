package agent

import(
 "context"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"

 "github.com/ichiken26/cusic/internal/phonology"
 "github.com/ichiken26/cusic/internal/store"
 "github.com/ichiken26/cusic/internal/svp"
 "github.com/modelcontextprotocol/go-sdk/mcp"
)

type pronunciationInput struct{
 Track string `json:"track"`
 Source string `json:"source,omitempty"`
}
type svpApplyInput struct{
 Source string `json:"source"`
 SourceTrack string `json:"sourceTrack,omitempty"`
 Track string `json:"track"`
 Out string `json:"out,omitempty"`
 Confirm bool `json:"confirm,omitempty"`
}
func readSVPProposal(s *store.Store,in svpApplyInput)([]byte,svp.Summary,error){
 sourceTrack:=in.SourceTrack
 if sourceTrack==""{sourceTrack=in.Track}
 if sourceTrack==""||in.Source==""{return nil,svp.Summary{},errors.New("source SVP and track required")}
 original,err:=os.ReadFile(pathFromProject(in.Source));if err!=nil{return nil,svp.Summary{},err}
 p,err:=svp.PayloadFromStore(s,original,sourceTrack,in.Track);if err!=nil{return nil,svp.Summary{},err}
 return svp.Apply(original,p,sourceTrack)
}
func writeNewSVP(file string,data []byte)error{
 if err:=os.MkdirAll(filepath.Dir(file),0755);err!=nil{return err}
 f,err:=os.OpenFile(file,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0644)
 if err!=nil{return err}
 _,writeErr:=f.Write(data)
 closeErr:=f.Close()
 if writeErr!=nil||closeErr!=nil{
  os.Remove(file)
  if writeErr!=nil{return writeErr}
  return closeErr
 }
 return nil
}
func registerSVPTools(server *mcp.Server){
 mcp.AddTool(server,&mcp.Tool{
  Name:"vocal_phonology_analyze",
  Description:"Analyze Japanese singing pronunciation: vowel devoicing (i/u in voiceless consonant context or phrase end), mora-to-note duration, consonant preutterance at barline, including くゆらす. Uses existing SVP if source provided, otherwise cusic vocal notes. Read-only; does not fabricate acoustically measured onset or force vowels.",
 },func(ctx context.Context,req *mcp.CallToolRequest,in pronunciationInput)(*mcp.CallToolResult,response,error){
  if in.Track==""{return nil,response{},errors.New("track required")}
  if in.Source!=""{
   report,err:=svp.AnalyzeFile(pathFromProject(in.Source),in.Track)
   if err!=nil{return nil,response{},err}
   return output(report)
  }
  return withStore(func(s *store.Store)(any,error){
   t,err:=s.Track(in.Track);if err!=nil{return nil,err}
   if t.Kind!="vocal"{return nil,errors.New("phonology analysis requires a vocal track")}
   notes,err:=s.Notes(t.ID,0,0);if err!=nil{return nil,err}
   settings,err:=s.Settings();if err!=nil{return nil,err}
   samples:=make([]phonology.Note,0,len(notes))
   for _,n:=range notes{samples=append(samples,phonology.Note{Tick:n.Tick,Duration:n.Duration,Lyric:n.Lyric,Pitch:n.Pitch})}
   return phonology.Analyze(samples,float64(settings.Tempo),settings.MeterNum),nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{
  Name:"svp_clone_plan",
  Description:"Preview cloning an existing SVP Teto track, adding cusic-edited notes to a NEW clone, and muting the original. Report Japanese phonetic timing risks; no files modified. Original SVP and singer/retakes remain intact.",
 },func(ctx context.Context,req *mcp.CallToolRequest,in svpApplyInput)(*mcp.CallToolResult,response,error){
  return withStore(func(s *store.Store)(any,error){
   _,summary,err:=readSVPProposal(s,in);if err!=nil{return nil,err}
   return map[string]any{"valid":true,"preview":summary,"source":in.Source,
    "note":"A new SVP copy is generated only via svp_clone_apply; the existing original is never overwritten. Acoustic alignment is heuristic; listen in SynthV."},nil
  })
 })
 mcp.AddTool(server,&mcp.Tool{
  Name:"svp_clone_apply",
  Description:"Clone the original Teto voice track into a NEW SVP file, apply cusic vocal notes on the cloned track and MUTE original. Original SVP always left intact, with original tuning/retakes and all other tracks. Requires confirm=true.",
 },func(ctx context.Context,req *mcp.CallToolRequest,in svpApplyInput)(*mcp.CallToolResult,response,error){
  if !in.Confirm{return nil,response{},errors.New("svp_clone_apply requires confirm=true after reviewing plan")}
  if in.Out==""||!strings.EqualFold(filepath.Ext(in.Out),".svp"){return nil,response{},errors.New("out must be a new .svp filename")}
  out,err:=outputPath(in.Out);if err!=nil{return nil,response{},err}
  return withStore(func(s *store.Store)(any,error){
   result,summary,err:=readSVPProposal(s,in);if err!=nil{return nil,err}
   if err=writeNewSVP(out,result);err!=nil{return nil,fmt.Errorf("SVP output could not be written: %w",err)}
   return map[string]any{"file":out,"result":summary,"originalUnchanged":true,"note":"Original muted only inside this new SVP. Open new output in SynthV to check phoneme timing and singer."},nil
  })
 })
}
