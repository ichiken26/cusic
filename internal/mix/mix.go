// Package mix safely modifies static mixer parameters in a COPY of an existing
// Cubase DAWproject. All original ZIP member bytes except project.xml are kept.
package mix
import(
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
 "path/filepath"
 "strconv"
 "strings"
)
type Band struct{
 Type string `json:"type"`
 Freq float64 `json:"freq"`
 Gain float64 `json:"gain"`
 Q float64 `json:"q"`
}
type Track struct{
 Name string `json:"name"`
 FaderDB *float64 `json:"faderDb,omitempty"`
 Pan *float64 `json:"pan,omitempty"`
 EQ []Band `json:"eq,omitempty"`
}
type Plan struct{Tracks []Track `json:"tracks"`}
func Validate(p Plan)error{
 if len(p.Tracks)==0||len(p.Tracks)>256{return errors.New("specify 1..256 mix tracks")}
 seen:=map[string]bool{}
 validBand:=map[string]bool{"highPass":true,"lowPass":true,"bandPass":true,"highShelf":true,"lowShelf":true,"bell":true,"notch":true}
 for _,tr:=range p.Tracks{
  if tr.Name==""||seen[tr.Name]{return fmt.Errorf("blank or duplicate track %q",tr.Name)}
  seen[tr.Name]=true
  if tr.FaderDB==nil&&tr.Pan==nil&&len(tr.EQ)==0{return fmt.Errorf("track %s has no mix changes",tr.Name)}
  if tr.FaderDB!=nil&&(math.IsNaN(*tr.FaderDB)||math.IsInf(*tr.FaderDB,0)||*tr.FaderDB< -60||*tr.FaderDB>18){return fmt.Errorf("invalid fader dB for %s",tr.Name)}
  if tr.Pan!=nil&&(math.IsNaN(*tr.Pan)||math.IsInf(*tr.Pan,0)||*tr.Pan< -1||*tr.Pan>1){return fmt.Errorf("pan outside [-1,1] on %s",tr.Name)}
  if len(tr.EQ)>16{return errors.New("too many EQ bands")}
  for _,b:=range tr.EQ{
   if !validBand[b.Type]||math.IsNaN(b.Freq)||math.IsNaN(b.Gain)||math.IsNaN(b.Q)||b.Freq<20||b.Freq>20000||b.Gain< -24||b.Gain>24||b.Q<0.1||b.Q>24{return fmt.Errorf("invalid EQ band for %s",tr.Name)}
  }
 }
 return nil
}
type node struct{
 Name xml.Name
 Attr []xml.Attr
 Children []any
}
func parse(data []byte)(*node,error){
 d:=xml.NewDecoder(bytes.NewReader(data))
 stack:=[]*node{};var root *node
 for{
  tok,err:=d.Token();if err==io.EOF{break};if err!=nil{return nil,err}
  switch t:=tok.(type){
  case xml.StartElement:
   v:=&node{Name:t.Name,Attr:append([]xml.Attr(nil),t.Attr...),Children:[]any{}}
   if len(stack)>0{p:=stack[len(stack)-1];p.Children=append(p.Children,v)}else{root=v}
   stack=append(stack,v)
  case xml.EndElement:
   if len(stack)==0{return nil,errors.New("invalid DAWproject XML")};stack=stack[:len(stack)-1]
  case xml.CharData:if len(stack)>0{p:=stack[len(stack)-1];p.Children=append(p.Children,append(xml.CharData(nil),t...))}
  case xml.Comment:if len(stack)>0{p:=stack[len(stack)-1];p.Children=append(p.Children,append(xml.Comment(nil),t...))}
  }
 }
 if root==nil||len(stack)!=0{return nil,errors.New("invalid project.xml")}
 return root,nil
}
func (n *node) child(name string)*node{
 for _,v:=range n.Children{if c,ok:=v.(*node);ok&&c.Name.Local==name{return c}}
 return nil
}
func (n *node) attr(name string)string{
 for _,a:=range n.Attr{if a.Name.Local==name{return a.Value}}
 return ""
}
func (n *node) setAttr(key,value string){
 for i:=range n.Attr{if n.Attr[i].Name.Local==key{n.Attr[i].Value=value;return}}
 n.Attr=append(n.Attr,xml.Attr{Name:xml.Name{Local:key},Value:value})
}
func element(name string)*node{return &node{Name:xml.Name{Local:name}}}
func param(name string,val float64,unit string)*node{
 e:=element(name);e.setAttr("value",strconv.FormatFloat(val,'f',-1,64));e.setAttr("unit",unit);return e
}
func (n *node) write(enc *xml.Encoder)error{
 start:=xml.StartElement{Name:n.Name,Attr:n.Attr}
 if err:=enc.EncodeToken(start);err!=nil{return err}
 for _,v:=range n.Children{
  if child,ok:=v.(*node);ok{if err:=child.write(enc);err!=nil{return err}}else{
   switch val:=v.(type){
   case xml.CharData:if err:=enc.EncodeToken(val);err!=nil{return err}
   case xml.Comment:if err:=enc.EncodeToken(val);err!=nil{return err}
   }
  }
 }
 return enc.EncodeToken(start.End())
}
func makeEQ(track string,bands []Band)*node{
 hash:=sha256.Sum256([]byte(track))
 id:="cusic_eq_"+hex.EncodeToString(hash[:6])
 eq:=element("Equalizer");eq.setAttr("id",id);eq.setAttr("name","cusic EQ");eq.setAttr("deviceRole","audioFX");eq.setAttr("enabled","true")
 for i,b:=range bands{
  band:=element("Band");band.setAttr("type",b.Type);band.setAttr("order",strconv.Itoa(i))
  band.Children=append(band.Children,param("Freq",b.Freq,"hertz"))
  if b.Type=="bell"||b.Type=="highShelf"||b.Type=="lowShelf"{band.Children=append(band.Children,param("Gain",b.Gain,"decibel"))}
  band.Children=append(band.Children,param("Q",b.Q,"linear"))
  on:=element("Enabled");on.setAttr("value","true");band.Children=append(band.Children,on)
  eq.Children=append(eq.Children,band)
 }
 return eq
}
func modify(root *node,plan Plan)error{
 if root.Name.Local!="Project"{return errors.New("not a DAWproject XML")}
 changes:=map[string]Track{};for _,tr:=range plan.Tracks{changes[tr.Name]=tr}
 found:=map[string]int{}
 var visit func(*node)error
 visit=func(n *node)error{
  if n.Name.Local=="Track"{
   if tr,ok:=changes[n.attr("name")];ok{
    found[tr.Name]++
    if found[tr.Name]>1{return fmt.Errorf("ambiguous track named %s",tr.Name)}
    channel:=n.child("Channel");if channel==nil{return fmt.Errorf("track %s has no Channel",tr.Name)}
    if tr.FaderDB!=nil{
     v:=math.Pow(10,*tr.FaderDB/20)
     p:=channel.child("Volume");if p==nil{return fmt.Errorf("track %s has no Volume parameter",tr.Name)}
     p.setAttr("value",strconv.FormatFloat(v,'f',8,64))
    }
    if tr.Pan!=nil{
     p:=channel.child("Pan");if p==nil{return fmt.Errorf("track %s has no Pan parameter",tr.Name)}
     p.setAttr("value",strconv.FormatFloat((*tr.Pan+1)/2,'f',8,64))
    }
    if len(tr.EQ)>0{
     devices:=channel.child("Devices")
     if devices==nil{
      devices=element("Devices")
      channel.Children=append([]any{devices},channel.Children...)
     }
     retained:=[]any{}
     for _,item:=range devices.Children{
      if el,ok:=item.(*node);ok&&el.Name.Local=="Equalizer"&&el.attr("name")=="cusic EQ"{continue}
      retained=append(retained,item)
     }
     devices.Children=append(retained,makeEQ(tr.Name,tr.EQ))
    }
   }
  }
  for _,item:=range n.Children{if c,ok:=item.(*node);ok{if err:=visit(c);err!=nil{return err}}}
  return nil
 }
 if err:=visit(root);err!=nil{return err}
 for name:=range changes{if found[name]!=1{return fmt.Errorf("mix target track %q not found",name)}}
 return nil
}
// Apply writes a NEW DAWproject archive. Existing VST plugin-state and WAV
// payloads are copied as-is. Only XML static values and an added cusic EQ change.
func Apply(source,destination string,plan Plan)error{
 if err:=Validate(plan);err!=nil{return err}
 if source==destination{return errors.New("source and destination must differ")}
 if _,err:=os.Stat(destination);err==nil{return errors.New("destination exists; refusing overwrite")}else if !errors.Is(err,os.ErrNotExist){return err}
 r,err:=zip.OpenReader(source);if err!=nil{return err};defer r.Close()
 var projectBytes []byte
 for _,z:=range r.File{if z.Name=="project.xml"{
  if z.UncompressedSize64>64<<20{return errors.New("project.xml exceeds 64MiB")}
  f,e:=z.Open();if e!=nil{return e}
  projectBytes,e=io.ReadAll(io.LimitReader(f,64<<20+1));f.Close();if e!=nil{return e}
 }}
 if len(projectBytes)==0{return errors.New("missing project.xml")}
 project,err:=parse(projectBytes);if err!=nil{return err}
 if err=modify(project,plan);err!=nil{return err}
 outXML:=&bytes.Buffer{};outXML.WriteString(xml.Header)
 enc:=xml.NewEncoder(outXML)
 if err=project.write(enc);err!=nil{return err};if err=enc.Flush();err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(destination),0755);err!=nil{return err}
 tmp,err:=os.CreateTemp(filepath.Dir(destination),".cusic-mix-*");if err!=nil{return err}
 defer os.Remove(tmp.Name())
 writer:=zip.NewWriter(tmp)
 for _,part:=range r.File{
  if strings.HasPrefix(part.Name,"../"){writer.Close();tmp.Close();return errors.New("unsafe ZIP member")}
  dest,e:=writer.CreateHeader(&part.FileHeader);if e!=nil{writer.Close();tmp.Close();return e}
  if part.Name=="project.xml"{_,e=dest.Write(outXML.Bytes())}else{
   src,e2:=part.Open();if e2!=nil{writer.Close();tmp.Close();return e2}
   _,e=io.Copy(dest,src);src.Close()
  }
  if e!=nil{writer.Close();tmp.Close();return e}
 }
 if err=writer.Close();err!=nil{tmp.Close();return err}
 if err=tmp.Close();err!=nil{return err}
 return os.Rename(tmp.Name(),destination)
}


// Check validates both the requested mix and referenced DAWproject track names
// without changing or exporting anything.
func Check(source string,plan Plan)error{
 if err:=Validate(plan);err!=nil{return err}
 r,err:=zip.OpenReader(source);if err!=nil{return err};defer r.Close()
 for _,f:=range r.File{if f.Name=="project.xml"{
  if f.UncompressedSize64>64<<20{return errors.New("project.xml exceeds size limit")}
  rc,err:=f.Open();if err!=nil{return err}
  data,err:=io.ReadAll(io.LimitReader(rc,64<<20+1));rc.Close();if err!=nil{return err}
  root,err:=parse(data);if err!=nil{return err}
  return modify(root,plan)
 }}
 return errors.New("DAWproject has no project.xml")
}
