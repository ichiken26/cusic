package mix

// DAWproject plug-in operations work on a COPY of an exported session.
// A VST3's opaque State is never guessed from a parameter name. We can clone
// an existing, working device (including its State payload), edit explicit
// DAWproject parameter metadata, or attach externally supplied native state.
// None of this controls the open Cubase session or edits .cpr files.
import (
 "archive/zip"
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "path"
 "path/filepath"
 "strconv"
 "strings"
)

type CompressorSettings struct {
 AttackMS float64 `json:"attackMs"`
 ReleaseMS float64 `json:"releaseMs"`
 Ratio float64 `json:"ratio"`
 ThresholdDB float64 `json:"thresholdDb"`
 OutputGainDB float64 `json:"outputGainDb"`
}
type DeviceOperation struct {
 Track string `json:"track"`
 Action string `json:"action"` // insertCompressor, updateCompressor, cloneVst3, setParameters, replaceState
 PluginID string `json:"pluginId,omitempty"`
 FromTrack string `json:"fromTrack,omitempty"`
 FromPluginID string `json:"fromPluginId,omitempty"`
 Compressor *CompressorSettings `json:"compressor,omitempty"`
 Parameters map[string]float64 `json:"parameters,omitempty"`
 StateFile string `json:"stateFile,omitempty"`
}
type DevicePlan struct {
 Operations []DeviceOperation `json:"operations"`
}
type DeviceInfo struct {
 ID string `json:"id"`
 Type string `json:"type"`
 Name string `json:"name"`
 DeviceID string `json:"deviceId,omitempty"`
 Vendor string `json:"vendor,omitempty"`
 Role string `json:"role,omitempty"`
 StatePath string `json:"statePath,omitempty"`
 StatePresent bool `json:"statePresent"`
 Parameters map[string]string `json:"parameters,omitempty"`
}
type ChannelInfo struct {
 Track string `json:"track"`
 ContentType string `json:"contentType"`
 AudioChannels int `json:"audioChannels"`
 Destination string `json:"destination,omitempty"`
 Pan string `json:"pan,omitempty"`
 Plugins []DeviceInfo `json:"plugins"`
}
type DeviceReport struct {
 Tracks []ChannelInfo `json:"tracks"`
 Kontakt []ChannelInfo `json:"kontakt"`
 Warnings []string `json:"warnings"`
 Limitations string `json:"limitations"`
}

func readDAWProject(source string)(*zip.ReadCloser,*node,error){
 reader,err:=zip.OpenReader(source);if err!=nil{return nil,nil,err}
 var data []byte
 found:=0
 for _,f:=range reader.File{
  if f.Name!="project.xml"{continue}
  found++
  if found>1 || f.UncompressedSize64>64<<20{reader.Close();return nil,nil,errors.New("duplicate/oversized project.xml")}
  file,e:=f.Open();if e!=nil{reader.Close();return nil,nil,e}
  data,e=io.ReadAll(io.LimitReader(file,64<<20+1));file.Close()
  if e!=nil{reader.Close();return nil,nil,e}
 }
 if len(data)==0{reader.Close();return nil,nil,errors.New("missing project.xml")}
 project,err:=parse(data)
 if err!=nil{reader.Close();return nil,nil,err}
 if project.Name.Local!="Project"{reader.Close();return nil,nil,errors.New("not a DAWproject Project")}
 return reader,project,nil
}
func allTracks(root *node)[]*node{
 var out []*node
 var visit func(*node)
 visit=func(n *node){
  if n.Name.Local=="Track"{out=append(out,n)}
  for _,v:=range n.Children{if c,ok:=v.(*node);ok{visit(c)}}
 }
 if structure:=root.child("Structure");structure!=nil{visit(structure)}
 return out
}
func locateTrack(root *node,name string)(*node,error){
 var result *node
 for _,track:=range allTracks(root){
  if track.attr("name")==name{
   if result!=nil{return nil,fmt.Errorf("ambiguous DAWproject track %q",name)}
   result=track
  }
 }
 if result==nil{return nil,fmt.Errorf("DAWproject track %q not found",name)}
 if result.child("Channel")==nil{return nil,fmt.Errorf("track %q has no channel",name)}
 return result,nil
}
func pluginByID(track *node,id string)(*node,error){
 ch:=track.child("Channel");if ch==nil{return nil,errors.New("track missing Channel")}
 devices:=ch.child("Devices");if devices==nil{return nil,fmt.Errorf("track %s has no devices",track.attr("name"))}
 var found *node
 for _,v:=range devices.Children{
  if d,ok:=v.(*node);ok&&d.attr("id")==id{
   if found!=nil{return nil,fmt.Errorf("duplicate device id %q",id)}
   found=d
  }
 }
 if found==nil{return nil,fmt.Errorf("device id %q not found on %s",id,track.attr("name"))}
 return found,nil
}
func safeZipName(s string)bool{
 if s==""||strings.Contains(s,"\\")||strings.HasPrefix(s,"/")||strings.Contains(s,":")||path.Clean(s)!=s{return false}
 for _,part:=range strings.Split(s,"/"){if part==".."||part=="."||part==""{return false}}
 return true
}
func zipContains(r *zip.ReadCloser,name string)bool{
 for _,z:=range r.File{if z.Name==name{return true}}
 return false
}
func cloneNode(n *node)*node{
 result:=&node{Name:n.Name,Attr:append([]xml.Attr(nil),n.Attr...)}
 for _,part:=range n.Children{
  if sub,ok:=part.(*node);ok{result.Children=append(result.Children,cloneNode(sub))}else{
   switch v:=part.(type){
   case xml.CharData:result.Children=append(result.Children,append(xml.CharData(nil),v...))
   case xml.Comment:result.Children=append(result.Children,append(xml.Comment(nil),v...))
   }
  }
 }
 return result
}
func xmlBytes(root *node)([]byte,error){
 var buffer bytes.Buffer;buffer.WriteString(xml.Header)
 enc:=xml.NewEncoder(&buffer)
 if err:=root.write(enc);err!=nil{return nil,err}
 if err:=enc.Flush();err!=nil{return nil,err}
 return buffer.Bytes(),nil
}
func stateData(r *zip.ReadCloser,name string)([]byte,error){
 if !safeZipName(name){return nil,errors.New("unsafe plug-in state path")}
 var found *zip.File
 for _,z:=range r.File{if z.Name==name{
  if found!=nil{return nil,fmt.Errorf("ambiguous duplicate state file %q",name)}
  found=z
 }}
 if found==nil{return nil,fmt.Errorf("plug-in state %q missing from archive",name)}
 if found.UncompressedSize64>128<<20{return nil,errors.New("plug-in state exceeds 128 MiB")}
 in,err:=found.Open();if err!=nil{return nil,err}
 defer in.Close()
 return io.ReadAll(io.LimitReader(in,128<<20+1))
}
func stateRef(d *node)string{
 if state:=d.child("State");state!=nil{return state.attr("path")}
 return ""
}
func statePayload(r *zip.ReadCloser,d *node)([]byte,error){
 state:=d.child("State")
 if state==nil||state.attr("path")==""{return nil,errors.New("VST3 device lacks embedded State")}
 if state.attr("external")=="true"{return nil,errors.New("external plug-in State cannot be cloned or edited")}
 return stateData(r,state.attr("path"))
}
func freshID(root *node,seed string)string{
 sum:=sha256.Sum256([]byte(seed))
 base:="cusic_"+hex.EncodeToString(sum[:8])
 used:=map[string]bool{}
 var walk func(*node)
 walk=func(n *node){if id:=n.attr("id");id!=""{used[id]=true};for _,v:=range n.Children{if e,ok:=v.(*node);ok{walk(e)}}}
 walk(root)
 for i:=0;;i++{
  candidate:=base
  if i>0{candidate=fmt.Sprintf("%s_%d",base,i)}
  if !used[candidate]{return candidate}
 }
}
func remapIDs(n *node,prefix string){
 count:=0
 var visit func(*node)
 visit=func(v *node){
  if v.attr("id")!=""{v.setAttr("id",fmt.Sprintf("%s_%d",prefix,count));count++}
  for _,e:=range v.Children{if child,ok:=e.(*node);ok{visit(child)}}
 }
 visit(n)
}
func putDevice(channel *node,device *node){
 devices:=channel.child("Devices")
 if devices==nil{
  devices=element("Devices")
  channel.Children=append([]any{devices},channel.Children...)
 }
 devices.Children=append(devices.Children,device)
}
func validCompressor(c *CompressorSettings)error{
 if c==nil{return errors.New("compressor settings required")}
 values:=[]float64{c.AttackMS,c.ReleaseMS,c.Ratio,c.ThresholdDB,c.OutputGainDB}
 for _,v:=range values{if math.IsNaN(v)||math.IsInf(v,0){return errors.New("non-finite compressor value")}}
 if c.AttackMS<0.1||c.AttackMS>1000||c.ReleaseMS<1||c.ReleaseMS>5000||
   c.Ratio<1||c.Ratio>30||c.ThresholdDB< -100||c.ThresholdDB>0||
   c.OutputGainDB< -24||c.OutputGainDB>24{return errors.New("compressor out of range (attack 0.1..1000ms, release 1..5000ms, ratio 1..30, threshold -100..0dB, gain -24..24dB)")}
 return nil
}
func realDeviceParam(name string,value float64,unit,id string)*node{
 n:=param(name,value,unit);n.setAttr("id",id);return n
}
func compressorNode(id string,c *CompressorSettings)*node{
 n:=element("Compressor")
 n.setAttr("id",id);n.setAttr("deviceName","cusic Compressor");n.setAttr("deviceRole","audioFX");n.setAttr("loaded","true")
 enabled:=element("Enabled");enabled.setAttr("value","true");enabled.setAttr("id",id+"_enabled")
 n.Children=[]any{
  enabled,
  realDeviceParam("Attack",c.AttackMS,"milliseconds",id+"_attack"),
  realDeviceParam("OutputGain",c.OutputGainDB,"decibel",id+"_makeup"),
  realDeviceParam("Ratio",c.Ratio,"linear",id+"_ratio"),
  realDeviceParam("Release",c.ReleaseMS,"milliseconds",id+"_release"),
  realDeviceParam("Threshold",c.ThresholdDB,"decibel",id+"_threshold"),
 }
 return n
}
func mutateCompressor(device *node,c *CompressorSettings){
 vals:=map[string]float64{"Attack":c.AttackMS,"Release":c.ReleaseMS,"Ratio":c.Ratio,"Threshold":c.ThresholdDB,"OutputGain":c.OutputGainDB}
 for name,value:=range vals{
  n:=device.child(name)
  if n==nil{
   n=realDeviceParam(name,value,"linear",device.attr("id")+"_"+strings.ToLower(name))
   device.Children=append(device.Children,n)
  }else{n.setAttr("value",strconv.FormatFloat(value,'f',-1,64))}
 }
}
func validateDevicePlan(plan DevicePlan)error{
 if len(plan.Operations)==0||len(plan.Operations)>64{return errors.New("specify 1..64 device operations")}
 for _,op:=range plan.Operations{
  if strings.TrimSpace(op.Track)==""||len(op.Track)>256{return errors.New("track name required")}
  switch op.Action{
  case "insertCompressor":if err:=validCompressor(op.Compressor);err!=nil{return err}
  case "updateCompressor":if op.PluginID==""{return errors.New("updateCompressor needs pluginId")};if err:=validCompressor(op.Compressor);err!=nil{return err}
  case "cloneVst3":if op.FromTrack==""||op.FromPluginID==""{return errors.New("cloneVst3 needs fromTrack and fromPluginId")}
  case "setParameters":
   if op.PluginID==""||len(op.Parameters)==0||len(op.Parameters)>128{return errors.New("setParameters needs pluginId and 1..128 existing params")}
   for k,v:=range op.Parameters{if k==""||math.IsNaN(v)||math.IsInf(v,0){return errors.New("invalid parameter name/value")}}
  case "replaceState":
   if op.PluginID==""||op.StateFile==""{return errors.New("replaceState needs pluginId and stateFile")}
  default:return fmt.Errorf("unsupported device action %q",op.Action)
  }
 }
 return nil
}
func applyDevicePlan(root *node,archive *zip.ReadCloser,plan DevicePlan)(map[string][]byte,error){
 if err:=validateDevicePlan(plan);err!=nil{return nil,err}
 additions:=map[string][]byte{}
 for index,op:=range plan.Operations{
  track,err:=locateTrack(root,op.Track);if err!=nil{return nil,err}
  channel:=track.child("Channel")
  switch op.Action{
  case "insertCompressor":
   id:=freshID(root,fmt.Sprintf("compressor:%s:%d",op.Track,index))
   putDevice(channel,compressorNode(id,op.Compressor))
  case "updateCompressor":
   dev,err:=pluginByID(track,op.PluginID);if err!=nil{return nil,err}
   if dev.Name.Local!="Compressor"{return nil,fmt.Errorf("%q is not a generic Compressor",op.PluginID)}
   mutateCompressor(dev,op.Compressor)
  case "cloneVst3":
   from,err:=locateTrack(root,op.FromTrack);if err!=nil{return nil,err}
   dev,err:=pluginByID(from,op.FromPluginID);if err!=nil{return nil,err}
   if dev.Name.Local!="Vst3Plugin"||dev.attr("deviceID")==""||dev.attr("deviceRole")!="audioFX"{return nil,errors.New("only an exported Vst3Plugin audioFX with deviceID can be cloned")}
   data,err:=statePayload(archive,dev);if err!=nil{return nil,err}
   id:=freshID(root,fmt.Sprintf("clone:%s:%s:%d",op.Track,op.FromPluginID,index))
   next:=cloneNode(dev)
   remapIDs(next,id)
   dest:=fmt.Sprintf("plugin-states/%s.bin",id)
   if zipContains(archive,dest)||additions[dest]!=nil{return nil,errors.New("generated plug-in state path collision")}
   next.child("State").setAttr("path",dest)
   additions[dest]=data
   putDevice(channel,next)
  case "setParameters":
   dev,err:=pluginByID(track,op.PluginID);if err!=nil{return nil,err}
   if dev.Name.Local!="Vst3Plugin"{return nil,errors.New("setParameters currently supports only Vst3Plugin")}
   if dev.child("State")!=nil{return nil,errors.New("opaque VST3 State exists: XML parameter edits would not update its native preset. Use replaceState with a state exported from that plug-in.")}
   params:=dev.child("Parameters")
   if params==nil{return nil,errors.New("VST3 has no exported Parameters element")}
   for name,value:=range op.Parameters{
    var match *node
    for _,e:=range params.Children{
     param,ok:=e.(*node);if !ok{continue}
     if param.attr("id")==name||param.attr("name")==name{
      if match!=nil{return nil,fmt.Errorf("ambiguous parameter %q",name)}
      match=param
     }
    }
    if match==nil{return nil,fmt.Errorf("parameter %q not exported by plug-in",name)}
    if match.Name.Local!="RealParameter"{return nil,fmt.Errorf("parameter %q is not a RealParameter",name)}
    if v:=match.attr("min");v!=""{lower,e:=strconv.ParseFloat(v,64);if e!=nil||value<lower{return nil,fmt.Errorf("parameter %q below its minimum",name)}}
    if v:=match.attr("max");v!=""{upper,e:=strconv.ParseFloat(v,64);if e!=nil||value>upper{return nil,fmt.Errorf("parameter %q above its maximum",name)}}
    match.setAttr("value",strconv.FormatFloat(value,'f',-1,64))
   }
  case "replaceState":
   dev,err:=pluginByID(track,op.PluginID);if err!=nil{return nil,err}
   if dev.Name.Local!="Vst3Plugin"||dev.attr("deviceID")==""{return nil,errors.New("replaceState requires a Vst3Plugin with deviceID")}
   if _,err:=statePayload(archive,dev);err!=nil{return nil,err}
   f,err:=os.Open(op.StateFile);if err!=nil{return nil,err}
   info,err:=f.Stat()
   if err!=nil{f.Close();return nil,err}
   if info.Size()>128<<20||info.Size()<16{f.Close();return nil,errors.New("state file must be 16B..128MiB")}
   data,err:=io.ReadAll(io.LimitReader(f,128<<20+1));f.Close()
   if err!=nil{return nil,err}
   // Plugin-native State is opaque. Require user to supply a preset from the
   // *same VST3*. We do not pretend to convert knob values into vendor chunks.
   id:=freshID(root,fmt.Sprintf("state:%s:%s:%d",op.Track,op.PluginID,index))
   dest:=fmt.Sprintf("plugin-states/%s.bin",id)
   if zipContains(archive,dest)||additions[dest]!=nil{return nil,errors.New("state path collision")}
   dev.child("State").setAttr("path",dest)
   additions[dest]=data
  }
 }
 return additions,nil
}

// CheckDevices performs the full XML/asset validation without touching files.
func CheckDevices(source string,plan DevicePlan)error{
 reader,root,err:=readDAWProject(source);if err!=nil{return err}
 defer reader.Close()
 _,err=applyDevicePlan(root,reader,plan)
 return err
}
// ApplyDevices copies all original ZIP member contents unchanged, except for
// project.xml, and appends new state files under unique names. Never overwrites.
func ApplyDevices(source,destination string,plan DevicePlan)error{
 if source==destination{return errors.New("source and destination must differ")}
 if _,err:=os.Stat(destination);err==nil{return errors.New("output exists")}else if !errors.Is(err,os.ErrNotExist){return err}
 reader,root,err:=readDAWProject(source);if err!=nil{return err}
 defer reader.Close()
 states,err:=applyDevicePlan(root,reader,plan);if err!=nil{return err}
 projectXML,err:=xmlBytes(root);if err!=nil{return err}
 if err:=os.MkdirAll(filepath.Dir(destination),0755);err!=nil{return err}
 temp,err:=os.CreateTemp(filepath.Dir(destination),".cusic-plugin-*");if err!=nil{return err}
 defer os.Remove(temp.Name())
 writer:=zip.NewWriter(temp)
 seen:=map[string]bool{}
 for _,entry:=range reader.File{
  if !safeZipName(entry.Name)||seen[entry.Name]{writer.Close();temp.Close();return fmt.Errorf("unsafe or duplicate archive member %q",entry.Name)}
  seen[entry.Name]=true
  if entry.Name=="project.xml"{
   out,e:=writer.CreateHeader(&entry.FileHeader);if e!=nil{writer.Close();temp.Close();return e}
   if _,e=out.Write(projectXML);e!=nil{writer.Close();temp.Close();return e}
  }else{
   if e:=writer.Copy(entry);e!=nil{writer.Close();temp.Close();return e}
  }
 }
 for name,data:=range states{
  if !safeZipName(name)||seen[name]{writer.Close();temp.Close();return errors.New("state archive member collision")}
  out,e:=writer.Create(name);if e!=nil{writer.Close();temp.Close();return e}
  if _,e=out.Write(data);e!=nil{writer.Close();temp.Close();return e}
 }
 if err:=writer.Close();err!=nil{temp.Close();return err}
 if err:=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),destination)
}
func InspectDevices(source string)(DeviceReport,error){
 result:=DeviceReport{Tracks:[]ChannelInfo{},Kontakt:[]ChannelInfo{},Warnings:[]string{},
  Limitations:"Read-only DAWproject metadata; cannot decode Kontakt proprietary multi-output configuration or verify plug-ins installed in Cubase. L/R audio levels need a rendered WAV to diagnose."}
 r,root,err:=readDAWProject(source);if err!=nil{return result,err}
 defer r.Close()
 for _,tr:=range allTracks(root){
  ch:=tr.child("Channel");if ch==nil{continue}
  info:=ChannelInfo{Track:tr.attr("name"),ContentType:tr.attr("contentType"),Destination:ch.attr("destination"),
   Pan:"",Plugins:[]DeviceInfo{}}
  info.AudioChannels,_=strconv.Atoi(ch.attr("audioChannels"))
  if pan:=ch.child("Pan");pan!=nil{info.Pan=pan.attr("value")}
  if info.AudioChannels==1{result.Warnings=append(result.Warnings,fmt.Sprintf("%s: channel is mono (audioChannels=1)",info.Track))}
  if info.Destination==""&&ch.attr("role")!="master"{result.Warnings=append(result.Warnings,fmt.Sprintf("%s: no channel destination in DAWproject",info.Track))}
  kontakt:=false
  if devices:=ch.child("Devices");devices!=nil{
   for _,v:=range devices.Children{
    dev,ok:=v.(*node);if !ok{continue}
    if !strings.HasSuffix(dev.Name.Local,"Plugin")&&dev.Name.Local!="Compressor"&&dev.Name.Local!="Equalizer"{continue}
    p:=DeviceInfo{ID:dev.attr("id"),Type:dev.Name.Local,Name:dev.attr("deviceName"),
      DeviceID:dev.attr("deviceID"),Vendor:dev.attr("deviceVendor"),Role:dev.attr("deviceRole"),StatePath:stateRef(dev),Parameters:map[string]string{}}
    if p.Name==""{p.Name=dev.attr("name")}
    if p.StatePath!=""{p.StatePresent=zipContains(r,p.StatePath)}
    if params:=dev.child("Parameters");params!=nil{
     for _,node:=range params.Children{
      if e,ok:=node.(*node);ok{
       key:=e.attr("name");if key==""{key=e.attr("id")}
       if key!=""{p.Parameters[key]=e.attr("value")}
      }
     }
    }
    info.Plugins=append(info.Plugins,p)
    if strings.Contains(strings.ToLower(p.Name+" "+p.Vendor),"kontakt"){kontakt=true}
    if p.StatePath!=""&&!p.StatePresent{result.Warnings=append(result.Warnings,fmt.Sprintf("%s / %s: state asset missing from archive",info.Track,p.Name))}
   }
  }
  result.Tracks=append(result.Tracks,info)
  if kontakt{
   result.Kontakt=append(result.Kontakt,info)
   result.Warnings=append(result.Warnings,fmt.Sprintf("%s: Kontakt internal outputs (st.1/st.2, channel routing and pan) are not exposed as readable DAWproject settings. Inspect Kontakt Outputs and loaded instrument routing in Cubase.",info.Track))
  }
 }
 return result,nil
}
