// Package vsqx adapts a user-exported Piapro Studio / VOCALOID4 VSQX template.
// This preserves the installed voice bank's identifiers and editor parameters.
// It does not attempt to synthesize unknown V4X voicebank identifiers.
package vsqx

import (
 "bytes"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "sort"
 "strconv"
 "strings"
 "github.com/ichiken26/cusic/internal/store"
)
type elem struct {Name xml.Name;Attrs []xml.Attr;Text string;Children []*elem}
func parse(data []byte)(*elem,error){
 d:=xml.NewDecoder(bytes.NewReader(data)); stack:=[]*elem{}; var root *elem
 for{
  tok,err:=d.Token();if err==io.EOF{break};if err!=nil{return nil,err}
  switch v:=tok.(type){
  case xml.StartElement:
   n:=&elem{Name:v.Name,Attrs:v.Attr}
   if len(stack)==0{if root!=nil{return nil,errors.New("multiple VSQX roots")};root=n}else{p:=stack[len(stack)-1];p.Children=append(p.Children,n)}
   stack=append(stack,n)
  case xml.EndElement:
   if len(stack)==0{return nil,errors.New("unexpected closing element")};stack=stack[:len(stack)-1]
  case xml.CharData:if len(stack)>0{stack[len(stack)-1].Text+=string(v)}
  }
 }
 if root==nil||len(stack)!=0{return nil,errors.New("invalid empty VSQX")}
 return root,nil
}
func (e *elem)child(name string)*elem{for _,c:=range e.Children{if c.Name.Local==name{return c}};return nil}
func (e *elem)all(name string)[]*elem{out:=[]*elem{};for _,c:=range e.Children{if c.Name.Local==name{out=append(out,c)}};return out}
func (e *elem)text(name string)string{if c:=e.child(name);c!=nil{return strings.TrimSpace(c.Text)};return ""}
func (e *elem)set(name,value string){
 if c:=e.child(name);c!=nil{c.Text=value;return}
 e.Children=append(e.Children,&elem{Name:xml.Name{Space:e.Name.Space,Local:name},Text:value})
}
func (e *elem)encode(enc *xml.Encoder)error{
 // Preserve the template's root default namespace instead of asking Go's encoder
 // to repeatedly declare it on every descendant (or duplicate it on the root).
 name:=xml.Name{Local:e.Name.Local}
 attrs:=make([]xml.Attr,0,len(e.Attrs))
 for _,a:=range e.Attrs{
  switch {
  case a.Name.Space=="xmlns":a.Name=xml.Name{Local:"xmlns:"+a.Name.Local}
  case a.Name.Space=="http://www.w3.org/2001/XMLSchema-instance":a.Name=xml.Name{Local:"xsi:"+a.Name.Local}
  }
  attrs=append(attrs,a)
 }
 if err:=enc.EncodeToken(xml.StartElement{Name:name,Attr:attrs});err!=nil{return err}
 if e.Text!=""{if err:=enc.EncodeToken(xml.CharData(e.Text));err!=nil{return err}}
 for _,c:=range e.Children{if err:=c.encode(enc);err!=nil{return err}}
 return enc.EncodeToken(xml.EndElement{Name:name})
}
func shortPhoneme(s string)string{
 // Simple syllables. Complex Japanese lyrics need review in Piapro Studio.
 mapKana:=map[string]string{
  "あ":"a","い":"i","う":"M","え":"e","お":"o",
  "か":"k a","き":"k' i","く":"k M","け":"k e","こ":"k o",
  "さ":"s a","し":"S i","す":"s M","せ":"s e","そ":"s o",
  "た":"t a","ち":"tS i","つ":"ts M","て":"t e","と":"t o",
  "な":"n a","に":"J i","ぬ":"n M","ね":"n e","の":"n o",
  "は":"h a","ひ":"C i","ふ":"p\\ M","へ":"h e","ほ":"h o",
  "ま":"m a","み":"m' i","む":"m M","め":"m e","も":"m o",
  "や":"j a","ゆ":"j M","よ":"j o",
  "ら":"4 a","り":"4' i","る":"4 M","れ":"4 e","ろ":"4 o",
  "わ":"w a","を":"o","ん":"N",
  "が":"g a","ぎ":"g' i","ぐ":"g M","げ":"g e","ご":"g o",
  "ざ":"dz a","じ":"dZ i","ず":"dz M","ぜ":"dz e","ぞ":"dz o",
  "だ":"d a","ぢ":"dZ i","づ":"dz M","で":"d e","ど":"d o",
  "ば":"b a","び":"b' i","ぶ":"b M","べ":"b e","ぼ":"b o",
  "ぱ":"p a","ぴ":"p' i","ぷ":"p M","ぺ":"p e","ぽ":"p o",
  "きゃ":"k' a","きゅ":"k' M","きょ":"k' o",
  "しゃ":"S a","しゅ":"S M","しょ":"S o",
  "ちゃ":"tS a","ちゅ":"tS M","ちょ":"tS o",
  "じゃ":"dZ a","じゅ":"dZ M","じょ":"dZ o",
  "にゃ":"J a","にゅ":"J M","にょ":"J o",
  "みゃ":"m' a","みゅ":"m' M","みょ":"m' o",
  "りゃ":"4' a","りゅ":"4' M","りょ":"4' o",
 }
 // one note per Japanese mora is recommended. Unknown syllables use "a" as a
 // visible placeholder and must be tuned after import.
 s=strings.TrimSpace(s);if v,ok:=mapKana[s];ok{return v}
 if s=="ー"||s=="-" {return "a"}
 return "a"
}
func Export(template []byte,settings store.Settings,track store.Track,notes []store.Note)([]byte,error){
 root,err:=parse(template);if err!=nil{return nil,err}
 if root.Name.Local!="vsq4"||root.Name.Space!="http://www.yamaha.co.jp/vocaloid/schema/vsq4/"{return nil,errors.New("template must be a VOCALOID4 VSQX exported from Piapro Studio")}
 master:=root.child("masterTrack");if master==nil{return nil,errors.New("template has no masterTrack")}
 voices:=root.child("vVoiceTable");if voices==nil||len(voices.all("vVoice"))==0{return nil,errors.New("template lacks voicebank metadata")}
 tracks:=root.all("vsTrack")
 if len(tracks)!=1{return nil,fmt.Errorf("template must have exactly one vsTrack, got %d",len(tracks))}
 parts:=tracks[0].all("vsPart");if len(parts)!=1{return nil,errors.New("template must have one vocal region (vsPart)")}
 if settings.MeterDen<=0||settings.MeterNum<=0||settings.PPQ!=store.PPQ{return nil,errors.New("unsupported score resolution or meter")}
 if len(master.all("tempo"))!=1||len(master.all("timeSig"))!=1{return nil,errors.New("use a single-tempo, single-meter VSQX template")}
 master.set("seqName",settings.Name);master.set("resolution","480")
 m:=master.child("tempo");m.set("t","0");m.set("v",strconv.Itoa(settings.Tempo*100))
 sig:=master.child("timeSig");sig.set("m","0");sig.set("nu",strconv.Itoa(settings.MeterNum));sig.set("de",strconv.Itoa(settings.MeterDen))
 pre,_:=strconv.ParseInt(master.text("preMeasure"),10,64)
 if pre<1||pre>16{return nil,errors.New("invalid VSQX preMeasure; expected 1..16")}
 lead:=pre*int64(settings.MeterNum)*store.PPQ*4/int64(settings.MeterDen)
 part:=parts[0];part.set("t",strconv.FormatInt(lead,10))
 tracks[0].set("name",track.Name)
 // Preserve all singer, voice bank and style data. Do not keep old automation
 // because its timing may no longer match the newly generated notes.
 sorted:=append([]store.Note(nil),notes...)
 sort.Slice(sorted,func(i,j int)bool{return sorted[i].Tick<sorted[j].Tick})
 filtered:=[]*elem{}
 for _,c:=range part.Children{
  if c.Name.Local=="note"||c.Name.Local=="mCtrl"{continue}
  filtered=append(filtered,c)
 }
 part.Children=filtered
 last:=int64(0)
 for _,n:=range sorted{
  if n.TrackID!=track.ID{return nil,fmt.Errorf("note %s belongs to another track",n.ID)}
  if err:=store.Validate(store.InputNote{Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Velocity:n.Velocity});err!=nil{return nil,err}
  if n.Tick+n.Duration>last{last=n.Tick+n.Duration}
  noteElem:=&elem{Name:xml.Name{Space:root.Name.Space,Local:"note"}}
  noteElem.set("t",strconv.FormatInt(n.Tick,10))
  noteElem.set("dur",strconv.FormatInt(n.Duration,10))
  noteElem.set("n",strconv.Itoa(n.Pitch))
  noteElem.set("v",strconv.Itoa(n.Velocity))
  lyric:=n.Lyric;if lyric==""{lyric="あ"}
  noteElem.set("y",lyric);noteElem.set("p",shortPhoneme(lyric))
  part.Children=append(part.Children,noteElem)
 }
 part.set("playTime",strconv.FormatInt(last+store.PPQ*4,10))
 out:=&bytes.Buffer{};out.WriteString(xml.Header)
 encoder:=xml.NewEncoder(out)
 if err:=root.encode(encoder);err!=nil{return nil,err};if err:=encoder.Flush();err!=nil{return nil,err}
 return out.Bytes(),nil
}
