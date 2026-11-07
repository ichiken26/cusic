// Package dawproject is an intentionally scoped DAWproject 1.0 exchange adapter.
// It handles arrangement note clips and aligned WAV audio stems. It does NOT
// roundtrip arbitrary VST state, folders, clip expressions or controller curves.
package dawproject

import(
 "archive/zip"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "path"
 "path/filepath"
 "sort"
 "strings"
 "github.com/ichiken26/cusic/internal/midifile"
 "github.com/ichiken26/cusic/internal/store"
)
type AudioFile struct{Name,AbsolutePath string;SampleRate,Channels int;Duration float64}
type ImportedAudio struct{Name,ArchivePath string}
type Imported struct{Song midifile.Song;Audio []ImportedAudio}
type xmlProject struct{
 XMLName xml.Name `xml:"Project"`
 Version string `xml:"version,attr"`
 App app `xml:"Application"`
 Transport transport `xml:"Transport"`
 Structure structure `xml:"Structure"`
 Arrangement arrangement `xml:"Arrangement"`
 Scenes string `xml:"Scenes"`
}
type app struct{Name string `xml:"name,attr"`;Version string `xml:"version,attr"`}
type transport struct{Tempo tempo `xml:"Tempo"`;Signature signature `xml:"TimeSignature"`}
type tempo struct{Value float64 `xml:"value,attr"`;Min float64 `xml:"min,attr"`;Max float64 `xml:"max,attr"`;Unit string `xml:"unit,attr"`;ID string `xml:"id,attr"`;Name string `xml:"name,attr"`}
type signature struct{Num int `xml:"numerator,attr"`;Den int `xml:"denominator,attr"`;ID string `xml:"id,attr"`}
type structure struct{Tracks []xmlTrack `xml:"Track"`}
type xmlTrack struct{
 ContentType string `xml:"contentType,attr"`
 Loaded bool `xml:"loaded,attr"`
 ID string `xml:"id,attr"`
 Name string `xml:"name,attr"`
 Channel channel `xml:"Channel"`
 Children []xmlTrack `xml:"Track"`
}
type channel struct{
 AudioChannels int `xml:"audioChannels,attr"`
 Role string `xml:"role,attr"`
 ID string `xml:"id,attr"`
 Destination string `xml:"destination,attr,omitempty"`
 Solo bool `xml:"solo,attr"`
 Mute boolParam `xml:"Mute"`
 Pan realParam `xml:"Pan"`
 Volume realParam `xml:"Volume"`
}
type boolParam struct{Value bool `xml:"value,attr"`;ID string `xml:"id,attr"`;Name string `xml:"name,attr"`}
type realParam struct{
 Value float64 `xml:"value,attr"`
 Min float64 `xml:"min,attr"`
 Max float64 `xml:"max,attr"`
 Unit string `xml:"unit,attr"`
 ID string `xml:"id,attr"`
 Name string `xml:"name,attr"`
}
type arrangement struct{ID string `xml:"id,attr"`;Lanes lanes `xml:"Lanes"`}
type lanes struct{
 ID string `xml:"id,attr"`
 TimeUnit string `xml:"timeUnit,attr,omitempty"`
 Track string `xml:"track,attr,omitempty"`
 Lanes []lanes `xml:"Lanes"`
 Clips *clips `xml:"Clips,omitempty"`
 Notes *noteList `xml:"Notes,omitempty"`
 Audio *audio `xml:"Audio,omitempty"`
 Warps *warps `xml:"Warps,omitempty"`
}
type clips struct{ID string `xml:"id,attr"`;Items []clip `xml:"Clip"`}
type clip struct{
 Name string `xml:"name,attr,omitempty"`
 Time float64 `xml:"time,attr"`
 Duration float64 `xml:"duration,attr"`
 PlayStart float64 `xml:"playStart,attr"`
 Notes *noteList `xml:"Notes,omitempty"`
 Warps *warps `xml:"Warps,omitempty"`
 Audio *audio `xml:"Audio,omitempty"`
 Clips *clips `xml:"Clips,omitempty"`
 Lanes *lanes `xml:"Lanes,omitempty"`
}
type noteList struct{ID string `xml:"id,attr"`;Notes []xmlNote `xml:"Note"`}
type xmlNote struct{
 Time float64 `xml:"time,attr"`
 Duration float64 `xml:"duration,attr"`
 Channel int `xml:"channel,attr"`
 Key int `xml:"key,attr"`
 Vel float64 `xml:"vel,attr"`
 Rel float64 `xml:"rel,attr"`
}
type warps struct{
 ID string `xml:"id,attr"`
 ContentTimeUnit string `xml:"contentTimeUnit,attr"`
 TimeUnit string `xml:"timeUnit,attr"`
 Audio audio `xml:"Audio"`
 Points []warp `xml:"Warp"`
}
type warp struct{Time float64 `xml:"time,attr"`;ContentTime float64 `xml:"contentTime,attr"`}
type audio struct{
 ID string `xml:"id,attr"`
 Channels int `xml:"channels,attr"`
 Duration float64 `xml:"duration,attr"`
 SampleRate int `xml:"sampleRate,attr"`
 Algorithm string `xml:"algorithm,attr,omitempty"`
 File audioFileRef `xml:"File"`
}
type audioFileRef struct{Path string `xml:"path,attr"`}
func basicChannel(id,dest string)channel{
 return channel{AudioChannels:2,Role:"regular",ID:id,Destination:dest,
  Mute:boolParam{Value:false,ID:id+"mute",Name:"Mute"},
  Pan:realParam{Value:0.5,Min:0,Max:1,Unit:"normalized",ID:id+"pan",Name:"Pan"},
  Volume:realParam{Value:1,Min:0,Max:2,Unit:"linear",ID:id+"vol",Name:"Volume"}}
}
func WriteFile(file string,s store.Settings,tracks []store.Track,notes map[string][]store.Note,assets []AudioFile)error{
 if s.Tempo<20||s.Tempo>400||s.MeterNum<1||s.MeterDen<1{return errors.New("invalid tempo/meter")}
 if len(tracks)+len(assets)>512{return errors.New("too many DAWproject tracks")}
 if _,err:=os.Stat(file);err==nil{return errors.New("output already exists")}else if !errors.Is(err,os.ErrNotExist){return err}
 if err:=os.MkdirAll(filepath.Dir(file),0755);err!=nil{return err}
 temp,err:=os.CreateTemp(filepath.Dir(file),".cusic-daw-*");if err!=nil{return err}
 defer os.Remove(temp.Name())
 z:=zip.NewWriter(temp)
 p:=xmlProject{Version:"1.0",App:app{Name:"cusic",Version:"0.2.0"},Transport:transport{
   Tempo:tempo{Value:float64(s.Tempo),Min:20,Max:400,Unit:"bpm",ID:"tempo",Name:"Tempo"},
   Signature:signature{Num:s.MeterNum,Den:s.MeterDen,ID:"meter"},
 },Arrangement:arrangement{ID:"arrangement",Lanes:lanes{ID:"timeline",TimeUnit:"beats",Lanes:[]lanes{}}}}
 master:=xmlTrack{ContentType:"audio notes",Loaded:true,ID:"master",Name:"Master",
  Channel:basicChannel("master_channel","")}
 master.Channel.Role="master";p.Structure.Tracks=append(p.Structure.Tracks,master)
 for index,tr:=range tracks{
  id:="track_"+tr.ID
  item:=xmlTrack{ContentType:"notes",Loaded:true,ID:id,Name:tr.Name,Channel:basicChannel("channel_"+tr.ID,"master_channel")}
  p.Structure.Tracks=append(p.Structure.Tracks,item)
  ns:=notes[tr.ID];ordered:=append([]store.Note(nil),ns...)
  sort.Slice(ordered,func(i,j int)bool{return ordered[i].Tick<ordered[j].Tick})
  list:=noteList{ID:fmt.Sprintf("notes_%d",index),Notes:[]xmlNote{}}
  end:=float64(0)
  for _,n:=range ordered {
   if err:=store.Validate(store.InputNote{Tick:n.Tick,Duration:n.Duration,Pitch:n.Pitch,Velocity:n.Velocity});err!=nil{return err}
   list.Notes=append(list.Notes,xmlNote{Time:float64(n.Tick)/float64(store.PPQ),Duration:float64(n.Duration)/float64(store.PPQ),Channel:0,Key:n.Pitch,Vel:float64(n.Velocity)/127,Rel:float64(n.Velocity)/127})
   candidate:=float64(n.Tick+n.Duration)/float64(store.PPQ);if candidate>end{end=candidate}
  }
  l:=lanes{ID:"lane_"+tr.ID,Track:id,Clips:&clips{ID:"clips_"+tr.ID,Items:[]clip{}}}
  if len(ordered)>0{l.Clips.Items=append(l.Clips.Items,clip{Time:0,Duration:end,PlayStart:0,Notes:&list})}
  p.Arrangement.Lanes.Lanes=append(p.Arrangement.Lanes.Lanes,l)
 }
 for index,a:=range assets{
  if a.SampleRate<8000||a.Channels<1||a.Duration<=0{return fmt.Errorf("invalid audio asset %q",a.Name)}
  id:=fmt.Sprintf("audio_%d",index)
  p.Structure.Tracks=append(p.Structure.Tracks,xmlTrack{ContentType:"audio",Loaded:true,ID:id,Name:a.Name,Channel:basicChannel("channel_"+id,"master_channel")})
  beats:=a.Duration*float64(s.Tempo)/60
  ref:=path.Join("audio",fmt.Sprintf("%03d-%s.wav",index,"stem"))
  wav:=audio{ID:"wave_"+id,Channels:a.Channels,Duration:a.Duration,SampleRate:a.SampleRate,File:audioFileRef{Path:ref}}
  item:=clip{Name:a.Name,Time:0,Duration:beats,PlayStart:0,Warps:&warps{ID:"warps_"+id,ContentTimeUnit:"seconds",TimeUnit:"beats",Audio:wav,Points:[]warp{{0,0},{beats,a.Duration}}}}
  p.Arrangement.Lanes.Lanes=append(p.Arrangement.Lanes.Lanes,lanes{ID:"lane_"+id,Track:id,Clips:&clips{ID:"clips_"+id,Items:[]clip{item}}})
 }
 p.Arrangement.Lanes.Lanes=append(p.Arrangement.Lanes.Lanes,lanes{ID:"master_lane",Track:"master",Clips:&clips{ID:"master_clips",Items:[]clip{}}})
 bytes,err:=xml.MarshalIndent(p,"","  ");if err!=nil{return err}
 projectEntry,err:=z.Create("project.xml");if err!=nil{return err}
 if _,err=projectEntry.Write(append([]byte(xml.Header),bytes...));err!=nil{return err}
 metadata,err:=z.Create("metadata.xml");if err!=nil{return err}
 if _,err=metadata.Write([]byte(xml.Header+`<MetaData><Title>`+xmlEsc(s.Name)+`</Title></MetaData>`));err!=nil{return err}
 for index,a:=range assets{
  entry,err:=z.Create(path.Join("audio",fmt.Sprintf("%03d-stem.wav",index)));if err!=nil{return err}
  src,err:=os.Open(a.AbsolutePath);if err!=nil{return err}
  _,err=io.Copy(entry,src);src.Close();if err!=nil{return err}
 }
 if err:=z.Close();err!=nil{return err}
 if err:=temp.Close();err!=nil{return err}
 return os.Rename(temp.Name(),file)
}
func xmlEsc(s string)string{
 b:=&strings.Builder{};xml.EscapeText(b,[]byte(s));return b.String()
}
func ReadFile(file string)(Imported,error){
 result:=Imported{Song:midifile.Song{Tempo:120,MeterNum:4,MeterDen:4,Tracks:[]store.ImportedTrack{}},Audio:[]ImportedAudio{}}
 r,err:=zip.OpenReader(file);if err!=nil{return result,err};defer r.Close()
 var input []byte
 for _,f:=range r.File{if f.Name=="project.xml"{
  if f.UncompressedSize64>32<<20{return result,errors.New("project.xml too large")}
  reader,e:=f.Open();if e!=nil{return result,e}
  input,e=io.ReadAll(io.LimitReader(reader,32<<20+1));reader.Close();if e!=nil{return result,e}
 }}
 if len(input)==0{return result,errors.New("not a DAWproject: missing project.xml")}
 var project xmlProject
 if err:=xml.Unmarshal(input,&project);err!=nil{return result,fmt.Errorf("DAWproject XML: %w",err)}
 if project.XMLName.Local!="Project"||project.Version!="1.0"{return result,errors.New("unsupported DAWproject format")}
 bpm:=int(math.Round(project.Transport.Tempo.Value))
 if bpm<20||bpm>400||project.Transport.Signature.Num<1||project.Transport.Signature.Den<1{return result,errors.New("invalid DAWproject tempo/meter")}
 result.Song.Tempo=bpm;result.Song.MeterNum=project.Transport.Signature.Num;result.Song.MeterDen=project.Transport.Signature.Den
 byID:=map[string]xmlTrack{}
 var indexTracks func([]xmlTrack)
 indexTracks=func(ts []xmlTrack){for _,t:=range ts{byID[t.ID]=t;indexTracks(t.Children)}}
 indexTracks(project.Structure.Tracks)
 var walk func(lanes,string,int)error
 notesByTrack:=map[string]*store.ImportedTrack{}
 order:=[]string{}
 seenAudio:=map[string]bool{}
 walk=func(l lanes,inheritedTrack string,depth int)error{
  if depth>64{return errors.New("DAWproject lane nesting exceeds 64")}
  trackID:=inheritedTrack
  if l.Track!=""{trackID=l.Track}
  tr,exists:=byID[trackID]
  if exists&&tr.Channel.Role!="master" {
   if strings.Contains(tr.ContentType,"notes") {
    if l.TimeUnit=="seconds"{return fmt.Errorf("note lane %q uses seconds, not beats",tr.Name)}
    part,ok:=notesByTrack[tr.ID]
    if !ok{part=&store.ImportedTrack{Name:tr.Name,Kind:"instrument",Notes:[]store.InputNote{}};notesByTrack[tr.ID]=part;order=append(order,tr.ID)}
    if l.Notes!=nil{if err:=gatherList(l.Notes,0,&part.Notes);err!=nil{return err}}
    if l.Clips!=nil{for _,c:=range l.Clips.Items{if err:=gatherNotes(c,c.Time,&part.Notes);err!=nil{return err}}}
   }
   if strings.Contains(tr.ContentType,"audio"){
    if l.Audio!=nil&&l.Audio.File.Path!=""{if err:=addArchiveAudio(tr.Name,l.Audio.File.Path,&result.Audio,seenAudio);err!=nil{return err}}
    if l.Warps!=nil&&l.Warps.Audio.File.Path!=""{if err:=addArchiveAudio(tr.Name,l.Warps.Audio.File.Path,&result.Audio,seenAudio);err!=nil{return err}}
    if l.Clips!=nil{for _,c:=range l.Clips.Items{for _,audioPath:=range clipAudioPaths(c){if err:=addArchiveAudio(tr.Name,audioPath,&result.Audio,seenAudio);err!=nil{return err}}}}
   }
  }
  for _,child:=range l.Lanes{if err:=walk(child,trackID,depth+1);err!=nil{return err}}
  return nil
 }
 if err:=walk(project.Arrangement.Lanes,"",0);err!=nil{return result,err}
 for _,id:=range order{result.Song.Tracks=append(result.Song.Tracks,*notesByTrack[id])}
 return result,nil
}
func addArchiveAudio(name,audioPath string,result *[]ImportedAudio,seen map[string]bool)error{
 if path.IsAbs(audioPath)||strings.Contains(audioPath,"..")||!strings.HasSuffix(strings.ToLower(audioPath),".wav"){return fmt.Errorf("unsafe or unsupported DAWproject audio path %q",audioPath)}
 key:=name+"\x00"+audioPath
 if !seen[key]{*result=append(*result,ImportedAudio{Name:name,ArchivePath:audioPath});seen[key]=true}
 return nil
}
func gatherList(list *noteList,offset float64,dest *[]store.InputNote)error{
 for _,n:=range list.Notes{
  if math.IsNaN(n.Time)||math.IsNaN(n.Duration)||math.IsNaN(n.Vel)||math.IsInf(n.Time,0)||math.IsInf(n.Duration,0)||math.IsInf(n.Vel,0){return errors.New("non-finite DAWproject note data")}
  tick:=int64(math.Round((offset+n.Time)*float64(store.PPQ)))
  duration:=int64(math.Round(n.Duration*float64(store.PPQ)))
  velocity:=int(math.Round(n.Vel*127))
  if velocity==0{velocity=100}
  item:=store.InputNote{Tick:tick,Duration:duration,Pitch:n.Key,Velocity:velocity}
  if err:=store.Validate(item);err!=nil{return fmt.Errorf("invalid DAWproject note at %.3f beats: %w",n.Time,err)}
  *dest=append(*dest,item)
 }
 return nil
}
func gatherNotes(c clip,clipStart float64,dest *[]store.InputNote)error{
 if c.PlayStart!=0{return errors.New("DAWproject note clip has nonzero playStart; flatten clips in Cubase before sync")}
 if c.Notes!=nil{if err:=gatherList(c.Notes,clipStart,dest);err!=nil{return err}}
 if c.Clips!=nil{for _,child:=range c.Clips.Items{if err:=gatherNotes(child,clipStart+child.Time,dest);err!=nil{return err}}}
 if c.Lanes!=nil{
  if c.Lanes.Notes!=nil{if err:=gatherList(c.Lanes.Notes,clipStart,dest);err!=nil{return err}}
  if c.Lanes.Clips!=nil{for _,child:=range c.Lanes.Clips.Items{if err:=gatherNotes(child,clipStart+child.Time,dest);err!=nil{return err}}}
 }
 return nil
}
func clipAudioPaths(c clip)[]string{
 out:=[]string{}
 if c.Audio!=nil&&c.Audio.File.Path!=""{out=append(out,c.Audio.File.Path)}
 if c.Warps!=nil&&c.Warps.Audio.File.Path!=""{out=append(out,c.Warps.Audio.File.Path)}
 if c.Clips!=nil{for _,child:=range c.Clips.Items{out=append(out,clipAudioPaths(child)...)}}
 if c.Lanes!=nil{
  if c.Lanes.Audio!=nil&&c.Lanes.Audio.File.Path!=""{out=append(out,c.Lanes.Audio.File.Path)}
  if c.Lanes.Warps!=nil&&c.Lanes.Warps.Audio.File.Path!=""{out=append(out,c.Lanes.Warps.Audio.File.Path)}
  if c.Lanes.Clips!=nil{for _,child:=range c.Lanes.Clips.Items{out=append(out,clipAudioPaths(child)...)}}
 }
 return out
}
// ExtractAudio extracts only a named embedded WAV into a NEW caller-provided path.
// No traversal and no overwrite. Imported media is never executed.
func ExtractAudio(projectFile,archivePath,out string)error{
 if path.IsAbs(archivePath)||strings.Contains(archivePath,"..")||!strings.HasSuffix(strings.ToLower(archivePath),".wav"){return errors.New("invalid embedded audio path")}
 if _,err:=os.Stat(out);err==nil{return errors.New("destination exists")}
 r,err:=zip.OpenReader(projectFile);if err!=nil{return err};defer r.Close()
 for _,f:=range r.File{
  if f.Name!=archivePath{continue}
  if f.UncompressedSize64>4<<30{return errors.New("audio exceeds 4GiB limit")}
  src,err:=f.Open();if err!=nil{return err};defer src.Close()
  dest,err:=os.OpenFile(out,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if err!=nil{return err}
  if _,err=io.Copy(dest,src);err!=nil{dest.Close();os.Remove(out);return err}
  return dest.Close()
 }
 return fmt.Errorf("embedded audio %q not found",archivePath)
}
