package agent

import (
 "context"
 "os"
 "path/filepath"
 "testing"
 "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPProjectFlow(t *testing.T) {
 root:=t.TempDir()
 t.Setenv("CUSIC_PROJECT",root)
 if err:=os.WriteFile(filepath.Join(root,"original.cpr"),[]byte("original"),0600);err!=nil{t.Fatal(err)}
 ctx:=context.Background()
 server:=NewServer()
 t1,t2:=mcp.NewInMemoryTransports()
 serverSession,err:=server.Connect(ctx,t1,nil);if err!=nil{t.Fatal(err)}
 defer serverSession.Close()
 client:=mcp.NewClient(&mcp.Implementation{Name:"cusic-smoke-test",Version:"1.0.0"},nil)
 session,err:=client.Connect(ctx,t2,nil);if err!=nil{t.Fatal(err)}
 defer session.Close()
 call:=func(name string,args map[string]any){
  t.Helper()
  res,err:=session.CallTool(ctx,&mcp.CallToolParams{Name:name,Arguments:args})
  if err!=nil{t.Fatalf("%s: %v",name,err)}
  if res.IsError{t.Fatalf("%s: MCP tool returned error: %+v",name,res)}
 }
 call("project_inventory",map[string]any{})
 call("project_initialize",map[string]any{"name":"Existing Song","tempo":168,"meterNum":4,"meterDen":4})
 call("project_status",map[string]any{})
 call("track_create",map[string]any{"name":"Bass","kind":"instrument","singer":""})
 call("track_inspect",map[string]any{"track":"Bass","fromBar":1,"toBar":4})
 call("patch_apply",map[string]any{
  "baseRevision":1,"message":"MCP creation","track":"Bass","fromBar":1,"toBar":1,
  "notes":[]map[string]any{{"tick":0,"duration":480,"pitch":40,"velocity":98,"lyric":""}},
 })
 call("history",map[string]any{})
 call("history_revert_head",map[string]any{"revision":2})
 call("vocal_source_status",map[string]any{})
 call("mix_analyze_stems",map[string]any{})
 original,err:=os.ReadFile(filepath.Join(root,"original.cpr"))
 if err!=nil||string(original)!="original"{t.Fatalf("original Cubase file changed: %v",err)}
}
