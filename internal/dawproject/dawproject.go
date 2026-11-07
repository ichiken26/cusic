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
 byID:=map[string]xmlTrack{};for _,t:=range project.Structure.Tracks{byID[t.ID]=t}
 for _,lane:=range project.Arrangement.Lanes.Lanes{
  tr,ok:=byID[lane.Track];if !ok||tr.Channel.Role=="master"{continue}
  parsed:=store.ImportedTrack{Name:tr.Name,Kind:"instrument",Notes:[]store.InputNote{}}
  if strings.Contains(tr.ContentType,"notes") {
   if lane.Clips!=nil{for _,clip:=range lane.Clips.Items{gatherNotes(clip,clip.Time,&parsed.Notes)}}
   if len(parsed.Notes)>0{result.Song.Tracks=append(result.Song.Tracks,parsed)}
  }
  if strings.Contains(tr.ContentType,"audio")&&lane.Clips!=nil{
   for _,clip:=range lane.Clips.Items{
    audioPath:=clipAudioPath(clip)
    if audioPath!=""{if strings.HasPrefix(audioPath,"/")||strings.Contains(audioPath,".."){return result,errors.New("unsafe DAWproject audio path")};result.Audio=append(result.Audio,ImportedAudio{Name:tr.Name,ArchivePath:audioPath})}
   }
  }
 }
 return result,nil
}
func gatherNotes(c clip,clipStart float64,dest *[]store.InputNote){
 if c.Notes!=nil{
  for _,n:=range c.Notes.Notes{
   tick:=int64(math.Round((clipStart+n.Time)*float64(store.PPQ)))
   duration:=int64(math.Round(n.Duration*float64(store.PPQ)))
   velocity:=int(math.Round(n.Vel*127));if velocity==0{velocity=100}
   item:=store.InputNote{Tick:tick,Duration:duration,Pitch:n.Key,Velocity:velocity}
   if store.Validate(item)==nil{*dest=append(*dest,item)}
  }
 }
 if c.Clips!=nil{for _,child:=range c.Clips.Items{gatherNotes(child,clipStart+child.Time,dest)}}
}
func clipAudioPath(c clip)string{
 if c.Audio!=nil{return c.Audio.File.Path}
 if c.Warps!=nil{return c.Warps.Audio.File.Path}
 if c.Clips!=nil{for _,child:=range c.Clips.Items{if p:=clipAudioPath(child);p!=""{return p}}}
 return ""
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
