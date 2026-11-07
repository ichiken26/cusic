package midifile

// Minimal Standard MIDI File (SMF 0/1) adapter. Deliberately conservative:
// unrepresentable features (tempo changes, SMPTE ticks) are rejected instead of
// silently corrupting an arrangement. Non-note events are not preserved.
import (
 "bytes"
 "encoding/binary"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "sort"

 "github.com/ichiken26/cusic/internal/store"
)

type Song struct { Tempo int; MeterNum int; MeterDen int; Tracks []store.ImportedTrack }
type event struct {Tick int64;Order int;Payload []byte}
type activeNote struct {Tick int64;Velocity int}

func ReadFile(path string)(Song,error){b,err:=os.ReadFile(path);if err!=nil{return Song{},err};return Read(b)}
func Read(data []byte)(Song,error){
 song:=Song{Tempo:120,MeterNum:4,MeterDen:4,Tracks:[]store.ImportedTrack{}}
 r:=bytes.NewReader(data)
 tag:=make([]byte,4)
 if _,err:=io.ReadFull(r,tag);err!=nil{return song,err}
 if string(tag)!="MThd"{return song,errors.New("not an SMF MIDI file")}
 var hdrLen uint32;if err:=binary.Read(r,binary.BigEndian,&hdrLen);err!=nil{return song,err}
 if hdrLen<6||hdrLen>4096{return song,errors.New("invalid MIDI header length")}
 hdr:=make([]byte,hdrLen);if _,err:=io.ReadFull(r,hdr);err!=nil{return song,err}
 format,nTracks,division:=binary.BigEndian.Uint16(hdr[:2]),binary.BigEndian.Uint16(hdr[2:4]),binary.BigEndian.Uint16(hdr[4:6])
 if format>1||division&0x8000!=0||division==0{return song,errors.New("supports SMF formats 0/1 with PPQ timing only")}
 if nTracks==0||nTracks>512{return song,errors.New("invalid MIDI track count")}
 tempoSeen,meterSeen:=false,false
 for tr:=0;tr<int(nTracks);tr++{
  if _,err:=io.ReadFull(r,tag);err!=nil{return song,err}
  if string(tag)!="MTrk"{return song,fmt.Errorf("track %d: missing MTrk",tr)}
  var length uint32;if err:=binary.Read(r,binary.BigEndian,&length);err!=nil{return song,err}
  if length>100*1024*1024{return song,errors.New("MIDI track exceeds 100MiB")}
  buf:=make([]byte,length);if _,err:=io.ReadFull(r,buf);err!=nil{return song,err}
  tracks,err:=parseTrack(buf,int(division),tr,&song,&tempoSeen,&meterSeen);if err!=nil{return song,fmt.Errorf("track %d: %w",tr,err)}
  song.Tracks=append(song.Tracks,tracks...)
 }
 if len(song.Tracks)==0{return song,errors.New("MIDI has no note tracks")}
 return song,nil
}
func parseTrack(data []byte,division,idx int,song *Song,tempoSeen,meterSeen *bool)([]store.ImportedTrack,error){
 at:=0;tick:=int64(0);running:=byte(0);name:=fmt.Sprintf("Track %d",idx+1)
 active:=map[int][]activeNote{}
 tracks:=map[byte][]store.InputNote{}
 for at<len(data){
  delta,e:=readVLQ(data,&at);if e!=nil{return nil,e};tick+=delta
  if at>=len(data){return nil,io.ErrUnexpectedEOF}
  status:=data[at]
  if status<0x80 {
   if running==0{return nil,errors.New("missing MIDI running status")}
   status=running
  }else{at++;if status<0xF0{running=status}else{running=0}}
  if status==0xFF {
   if at>=len(data){return nil,io.ErrUnexpectedEOF}
   kind:=data[at];at++;size,e:=readVLQ(data,&at);if e!=nil{return nil,e}
   if size>int64(len(data)-at){return nil,io.ErrUnexpectedEOF}
   raw:=data[at:at+int(size)];at+=int(size)
   switch kind {
   case 0x03:if len(raw)>0{name=string(raw)}
   case 0x51:
    if len(raw)!=3{return nil,errors.New("bad tempo meta")}
    us:=int(raw[0])<<16|int(raw[1])<<8|int(raw[2]);if us==0{return nil,errors.New("zero tempo")}
    bpm:=int(math.Round(60000000.0/float64(us)))
    if tick!=0 && (!*tempoSeen||bpm!=song.Tempo){return nil,errors.New("tempo map with changes is not supported")}
    if *tempoSeen&&bpm!=song.Tempo{return nil,errors.New("tempo map with changes is not supported")}
    song.Tempo=bpm;*tempoSeen=true
   case 0x58:
    if len(raw)<2||raw[1]>5{return nil,errors.New("bad time signature")}
    n,d:=int(raw[0]),1<<raw[1]
    if tick!=0&&(!*meterSeen||n!=song.MeterNum||d!=song.MeterDen){return nil,errors.New("meter changes are not supported")}
    if *meterSeen&&(n!=song.MeterNum||d!=song.MeterDen){return nil,errors.New("meter changes are not supported")}
    song.MeterNum, song.MeterDen=n,d;*meterSeen=true
   case 0x2F:at=len(data)
   }
   continue
  }
  if status==0xF0||status==0xF7 {
   size,e:=readVLQ(data,&at);if e!=nil{return nil,e}
   if size>int64(len(data)-at){return nil,io.ErrUnexpectedEOF};at+=int(size);continue
  }
  if status>=0xF0{return nil,fmt.Errorf("unsupported MIDI system event 0x%X",status)}
  kind:=status&0xF0;ch:=status&0x0F
  count:=2;if kind==0xC0||kind==0xD0{count=1}
  if at+count>len(data){return nil,io.ErrUnexpectedEOF}
  a:=data[at];b:=byte(0);if count==2{b=data[at+1]};at+=count
  if a>=128||b>=128{return nil,errors.New("invalid MIDI data byte")}
  key:=int(ch)*128+int(a)
  if kind==0x90&&b!=0 {
   active[key]=append(active[key],activeNote{Tick:tick,Velocity:int(b)})
  } else if kind==0x80||(kind==0x90&&b==0){
   queue:=active[key];if len(queue)==0{continue}
   on:=queue[0];active[key]=queue[1:]
   if tick>on.Tick{
    start:=rescale(on.Tick,division);end:=rescale(tick,division)
    if end<=start{end=start+1}
    tracks[ch]=append(tracks[ch],store.InputNote{Tick:start,Duration:end-start,Pitch:int(a),Velocity:on.Velocity})
   }
  }
 }
 for _,ns:=range active{if len(ns)!=0{return nil,errors.New("unclosed MIDI note-on")}}
 chans:=make([]int,0,len(tracks));for ch:=range tracks{chans=append(chans,int(ch))};sort.Ints(chans)
 result:=[]store.ImportedTrack{}
 for _,c:=range chans{
  ch:=byte(c);ns:=tracks[ch]
  if len(ns)==0{continue}
  sort.Slice(ns,func(i,j int)bool{if ns[i].Tick==ns[j].Tick{return ns[i].Pitch<ns[j].Pitch};return ns[i].Tick<ns[j].Tick})
  title:=name;if len(chans)>1{title=fmt.Sprintf("%s ch%d",name,c+1)}
  kind:="instrument";if c==9{kind="drums"}
  result=append(result,store.ImportedTrack{Name:title,Kind:kind,Notes:ns})
 }
 return result,nil
}
func rescale(tick int64,division int)int64{return (tick*store.PPQ+int64(division)/2)/int64(division)}
func readVLQ(b []byte,pos *int)(int64,error){
 var value int64
 for i:=0;i<4;i++{
  if *pos>=len(b){return 0,io.ErrUnexpectedEOF}
  v:=b[*pos];*pos++
  value=value<<7|int64(v&0x7F)
  if v&0x80==0{return value,nil}
 }
 return 0,errors.New("invalid VLQ")
}
func encodeVLQ(v int64)[]byte{
 var out [4]byte;out[3]=byte(v&0x7F);for i:=2;i>=0;i--{v>>=7;if v==0{return append([]byte(nil),out[i+1:]...)};out[i]=byte(v&0x7F)|0x80}
 return out[:]
}
func writeChunk(w *bytes.Buffer,tag string,data []byte){
 w.WriteString(tag);_ = binary.Write(w,binary.BigEndian,uint32(len(data)));w.Write(data)
}
func Write(settings store.Settings, tracks []store.Track, notes map[string][]store.Note)([]byte,error){
 if settings.Tempo<20||settings.Tempo>400{return nil,errors.New("invalid tempo")}
 if settings.MeterNum<1||settings.MeterNum>255||settings.MeterDen<1||settings.MeterDen>32||settings.MeterDen&(settings.MeterDen-1)!=0{return nil,errors.New("invalid meter")}
 if len(tracks)>511{return nil,errors.New("too many tracks")}
 out:=&bytes.Buffer{};hdr:=&bytes.Buffer{}
 _=binary.Write(hdr,binary.BigEndian,uint16(1));_=binary.Write(hdr,binary.BigEndian,uint16(len(tracks)+1));_=binary.Write(hdr,binary.BigEndian,uint16(store.PPQ))
 writeChunk(out,"MThd",hdr.Bytes())
 tempo:=60000000/settings.Tempo
 denPower:=byte(0);for 1<<denPower<settings.MeterDen{denPower++}
 conductor:=[]event{
  {Tick:0,Order:0,Payload:[]byte{0xFF,0x51,3,byte(tempo>>16),byte(tempo>>8),byte(tempo)}},
  {Tick:0,Order:0,Payload:[]byte{0xFF,0x58,4,byte(settings.MeterNum),denPower,24,8}},
 }
 writeChunk(out,"MTrk",makeTrack(conductor))
 for _,tr:=range tracks{
  evs:=[]event{}
  name:=[]byte(tr.Name);if len(name)>4096{return nil,errors.New("track name too long")}
  evs=append(evs,event{Tick:0,Order:0,Payload:append([]byte{0xFF,0x03},append(encodeVLQ(int64(len(name))),name...)...)})
  channel:=byte(0);if tr.Kind=="drums"{channel=9}
  for _,n:=range notes[tr.ID]{
   if err:=store.Validate(store.InputNote{Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Velocity:n.Velocity});err!=nil{return nil,err}
   if n.Tick>0x0FFFFFFF||n.Tick+n.Duration>0x0FFFFFFF{return nil,errors.New("MIDI tick exceeds SMF limit")}
   evs=append(evs,event{Tick:n.Tick,Order:2,Payload:[]byte{0x90|channel,byte(n.Pitch),byte(n.Velocity)}})
   evs=append(evs,event{Tick:n.Tick+n.Duration,Order:1,Payload:[]byte{0x80|channel,byte(n.Pitch),0}})
  }
  writeChunk(out,"MTrk",makeTrack(evs))
 }
 return out.Bytes(),nil
}
func makeTrack(evs []event)[]byte{
 sort.SliceStable(evs,func(i,j int)bool{if evs[i].Tick==evs[j].Tick{return evs[i].Order<evs[j].Order};return evs[i].Tick<evs[j].Tick})
 b:=&bytes.Buffer{};previous:=int64(0)
 for _,ev:=range evs{b.Write(encodeVLQ(ev.Tick-previous));b.Write(ev.Payload);previous=ev.Tick}
 b.Write([]byte{0,0xFF,0x2F,0});return b.Bytes()
}
