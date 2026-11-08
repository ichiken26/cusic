package main

import (
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestSetupGeneratesScopedConfigsAndPreservesOriginal(t *testing.T){
 root:=t.TempDir()
 source:=filepath.Join(root,"song.cpr")
 if err:=os.WriteFile(source,[]byte("untouched"),0644);err!=nil{t.Fatal(err)}
 if err:=setup([]string{root});err!=nil{t.Fatal(err)}
 claude,err:=os.ReadFile(filepath.Join(root,".mcp.json"));if err!=nil{t.Fatal(err)}
 var data map[string]any
 if err=json.Unmarshal(claude,&data);err!=nil{t.Fatal(err)}
 if _,ok:=data["mcpServers"];!ok{t.Fatal("missing Claude MCP")}
 codex,err:=os.ReadFile(filepath.Join(root,".codex","config.toml"));if err!=nil{t.Fatal(err)}
 if !strings.Contains(string(codex),"[mcp_servers.cusic]"){t.Fatal("missing Codex config")}
 if _,err=os.Stat(filepath.Join(root,"AGENTS.md"));err!=nil{t.Fatal(err)}
 if err:=setup([]string{root});err!=nil{t.Fatal(err)}
 codex2,err:=os.ReadFile(filepath.Join(root,".codex","config.toml"));if err!=nil{t.Fatal(err)}
 if string(codex)!=string(codex2){t.Fatal("duplicate Codex config")}
 original,_:=os.ReadFile(source);if string(original)!="untouched"{t.Fatal("Cubase project modified")}
}
