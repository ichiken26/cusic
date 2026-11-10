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
func shortPhoneme(s string)(string,bool){
 // VSQX/Piapro V4X special lyrics. Not every voicebank implements breath
 // recordings; the user must check the rendered voice.
 special:=strings.ToLower(strings.TrimSpace(s))
 switch special{
 case "っ","ッ","sil","[sil]","pau","[pau]":return "Sil",true
 case "br1","br2","br3","br4","br5":return special,true
 case "ー","-":return "-",true
 }

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
 s=strings.TrimSpace(s)
 // Normalize katakana to hiragana to reuse the same pronunciation table.
 runes:=[]rune(s);for i,r:=range runes{if r>=0x30A1&&r<=0x30F6{runes[i]=r-0x60}}
 normalized:=string(runes)
 if v,ok:=mapKana[normalized];ok{return v,true}
 return "",false
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
 var originalStyle *elem
 for _,node:=range part.Children{if node.Name.Local=="note" && node.child("nStyle")!=nil{originalStyle=node.child("nStyle");break}}
 filtered:=[]*elem{}
 for _,c:=range part.Children{
  if c.Name.Local=="note"||c.Name.Local=="mCtrl"{continue}
  filtered=append(filtered,c)
 }
 part.Children=filtered
 // Carry a note style from the Piapro-created template when available.
 if originalStyle==nil{
  originalStyle=&elem{Name:xml.Name{Local:"nStyle",Space:root.Name.Space}}
  defaults:=[]struct{Key string;Value string}{{"accent","50"},{"bendDep","8"},{"bendLen","0"},{"decay","50"},{"fallPort","0"},{"opening","127"},{"risePort","0"},{"vibLen","0"},{"vibType","0"}}
  for _,d:=range defaults{
   originalStyle.Children=append(originalStyle.Children,&elem{Name:xml.Name{Local:"v",Space:root.Name.Space},Attrs:[]xml.Attr{{Name:xml.Name{Local:"id"},Value:d.Key}},Text:d.Value})
  }
 }
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
  phoneme,ok:=shortPhoneme(lyric)
  if !ok{return nil,fmt.Errorf("unknown lyric %q on note %s: provide one hiragana/katakana mora per note (kanji must be converted to kana)",lyric,n.ID)}
  noteElem.set("y",lyric);noteElem.set("p",phoneme)
  noteElem.Children=append(noteElem.Children,originalStyle)
  part.Children=append(part.Children,noteElem)
 }
 part.set("playTime",strconv.FormatInt(last+store.PPQ*4,10))
 out:=&bytes.Buffer{};out.WriteString(xml.Header)
 encoder:=xml.NewEncoder(out)
 if err:=root.encode(encoder);err!=nil{return nil,err};if err:=encoder.Flush();err!=nil{return nil,err}
 return out.Bytes(),nil
}


// NoteEdit makes a narrowly scoped, non-destructive edit to a note already
// present in a VOCALOID4 VSQX. Onset and duration remain unchanged, so all
// original mCtrl events and note style curves remain at their original times.
type NoteEdit struct{
 Tick int64 `json:"tick"`
 ExpectedPitch int `json:"expectedPitch"`
 NewPitch *int `json:"newPitch,omitempty"`
 NewLyric *string `json:"newLyric,omitempty"`
}
func EditExisting(source []byte, edits []NoteEdit)([]byte,error){
 if len(edits)==0{return nil,errors.New("at least one edit required")}
 root,err:=parse(source);if err!=nil{return nil,err}
 if root.Name.Local!="vsq4"||root.Name.Space!="http://www.yamaha.co.jp/vocaloid/schema/vsq4/"{return nil,errors.New("VOCALOID4 VSQX required")}
 tracks:=root.all("vsTrack");if len(tracks)!=1{return nil,errors.New("expected exactly one vocal track")}
 parts:=tracks[0].all("vsPart");if len(parts)!=1{return nil,errors.New("expected exactly one part")}
 part:=parts[0]
 targets:=map[int64]*elem{}
 for _,n:=range part.all("note"){
  tick,e:=strconv.ParseInt(n.text("t"),10,64);if e!=nil{return nil,fmt.Errorf("invalid note tick: %w",e)}
  if _,exists:=targets[tick];exists{return nil,fmt.Errorf("multiple notes at tick %d; ambiguous",tick)}
  targets[tick]=n
 }
 seen:=map[int64]bool{}
 // Validate everything *before* making any edit.
 for _,edit:=range edits{
  if seen[edit.Tick]{return nil,fmt.Errorf("duplicate edit at tick %d",edit.Tick)}
  seen[edit.Tick]=true
  n,ok:=targets[edit.Tick];if !ok{return nil,fmt.Errorf("no existing note at tick %d",edit.Tick)}
  p,e:=strconv.Atoi(n.text("n"));if e!=nil||p!=edit.ExpectedPitch{return nil,fmt.Errorf("pitch mismatch at tick %d: expected %d, VSQX has %d",edit.Tick,edit.ExpectedPitch,p)}
  if edit.NewPitch==nil&&edit.NewLyric==nil{return nil,errors.New("every edit must change pitch or lyric")}
  if edit.NewPitch!=nil&&(*edit.NewPitch<0||*edit.NewPitch>127){return nil,fmt.Errorf("pitch out of range at tick %d",edit.Tick)}
  if edit.NewLyric!=nil{if _,ok:=shortPhoneme(*edit.NewLyric);!ok{return nil,fmt.Errorf("unsupported lyric %q",*edit.NewLyric)}}
 }
 for _,edit:=range edits{
  n:=targets[edit.Tick]
  if edit.NewPitch!=nil{n.set("n",strconv.Itoa(*edit.NewPitch))}
  if edit.NewLyric!=nil{p,_:=shortPhoneme(*edit.NewLyric);n.set("y",*edit.NewLyric);n.set("p",p)}
 }
 out:=&bytes.Buffer{};out.WriteString(xml.Header)
 encoder:=xml.NewEncoder(out);if err:=root.encode(encoder);err!=nil{return nil,err}
 if err:=encoder.Flush();err!=nil{return nil,err}
 return out.Bytes(),nil
}

func encodeVSQX(root *elem)([]byte,error){
 out:=&bytes.Buffer{}
 out.WriteString(xml.Header)
 encoder:=xml.NewEncoder(out)
 if err:=root.encode(encoder);err!=nil{return nil,err}
 if err:=encoder.Flush();err!=nil{return nil,err}
 return out.Bytes(),nil
}
// ExportForVoice reuses registered voicebank metadata, never guesses IDs.
func ExportForVoice(template []byte,voice string,allowFallback bool,settings store.Settings,track store.Track,notes []store.Note)([]byte,VoiceTemplate,error){
 resolved,selected,err:=SelectVoice(template,voice,allowFallback)
 if err!=nil{return nil,selected,err}
 data,err:=Export(resolved,settings,track,notes)
 return data,selected,err
}
