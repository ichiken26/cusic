// Package tuxguitar reads the ZIP/XML .tg format introduced by TuxGuitar 2.0.
// The legacy binary 1.x format is intentionally rejected with an actionable error.
// Lossy conversion: tablature fingering, effects, annotations are not carried by cusic's MIDI note model.
package tuxguitar

import (
 "archive/zip"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
 "strconv"
 "strings"
 "github.com/ichiken26/cusic/internal/midifile"
 "github.com/ichiken26/cusic/internal/store"
)
type file struct {
 XMLName xml.Name `xml:"TuxGuitarFile"`
 Version struct{Major int `xml:"major,attr"` } `xml:"TGVersion"`
 Song tgSong `xml:"TGSong"`
}
type tgSong struct {
 Name string `xml:"name"`
 Channels []channel `xml:"TGChannel"`
 Headers []header `xml:"TGMeasureHeader"`
 Tracks []track `xml:"TGTrack"`
}
type channel struct {
 ID int `xml:"id"`
 Bank int `xml:"bank"`
}
type header struct {
 Signature struct{Num int `xml:"numerator,attr"`; Den int `xml:"denominator,attr"` } `xml:"timeSignature"`
 Tempo int `xml:"tempo"`
}
type track struct {
 Name string `xml:"name"`
 ChannelID int `xml:"channelId"`
 Offset int `xml:"offset"`
 Strings []int `xml:"TGString"`
 Measures []measure `xml:"TGMeasure"`
}
type measure struct{ Beats []beat `xml:"TGBeat"` }
type beat struct {
 Start int64 `xml:"preciseStart"`
 Voices []voice `xml:"voice"`
}
type voice struct{
 Empty string `xml:"empty,attr"`
 Duration duration `xml:"duration"`
 Notes []note `xml:"note"`
}
type duration struct {
 Value int `xml:"value,attr"`
 Dotted string `xml:"dotted,attr"`
 Division struct{Enters int `xml:"enters,attr"`;Times int `xml:"times,attr"` } `xml:"divisionType"`
}
type note struct {
 Fret int `xml:"value,attr"`
 String int `xml:"string,attr"`
 Velocity int `xml:"velocity,attr"`
 Tie bool `xml:"tiedNote,attr"`
}
const (
 // Least common multiple of 64,3,5,6,7,9,10,11,12,13 multiplied by four.
 // This is precisely the timeline quantum TuxGuitar 2.x stores.
 wholePrecise int64 = 5765760
)
func ReadFile(path string)(midifile.Song,error){
 r,err:=zip.OpenReader(path);if err!=nil{return midifile.Song{},errors.New("cannot read .tg ZIP; for TuxGuitar 1.x export MIDI or upgrade the .tg file to 2.x")}
 defer r.Close()
 var version,content []byte
 if len(r.File)>20{return midifile.Song{},errors.New("too many TG archive entries")}
 for _,f:=range r.File {
  if f.Name!="version.txt" && f.Name!="content.xml"{continue}
  if f.UncompressedSize64>16<<20{return midifile.Song{},errors.New("TG XML exceeds 16MiB")}
  reader,e:=f.Open();if e!=nil{return midifile.Song{},e}
  b,e:=io.ReadAll(io.LimitReader(reader,16<<20+1));reader.Close();if e!=nil{return midifile.Song{},e}
  if f.Name=="version.txt"{version=b}else{content=b}
 }
 if !strings.HasPrefix(string(version),"TuxGuitar_file_format 2.")||len(content)==0{return midifile.Song{},errors.New("unsupported TG version: expects TuxGuitar 2.x ZIP with version.txt and content.xml")}
 var parsed file
 if err:=xml.Unmarshal(content,&parsed);err!=nil{return midifile.Song{},fmt.Errorf("TG content.xml: %w",err)}
 if parsed.XMLName.Local!="TuxGuitarFile"||parsed.Version.Major!=2{return midifile.Song{},errors.New("unrecognized TG XML root/version")}
 if len(parsed.Song.Headers)==0||len(parsed.Song.Tracks)==0{return midifile.Song{},errors.New("TG has no measure headers or tracks")}
 first:=parsed.Song.Headers[0]
 if first.Tempo<20||first.Tempo>400||first.Signature.Num<1||first.Signature.Num>32||first.Signature.Den<1{return midifile.Song{},errors.New("invalid TG tempo/time signature")}
 // cusic's current score is single tempo/meter; never silently shift a song.
 for _,h:=range parsed.Song.Headers {
  if h.Tempo!=first.Tempo||h.Signature!=first.Signature{return midifile.Song{},errors.New("TG tempo/meter changes require a tempo-map capable score; export one fixed-tempo passage instead")}
 }
 song:=midifile.Song{Tempo:first.Tempo,MeterNum:first.Signature.Num,MeterDen:first.Signature.Den,Tracks:[]store.ImportedTrack{}}
 channels:=map[int]channel{};for _,c:=range parsed.Song.Channels{channels[c.ID]=c}
 // A whole note is 4 quarter notes, and a TuxGuitar 2 song begins at 1 quarter.
 start:=wholePrecise/4
 for index,t:=range parsed.Song.Tracks {
  if len(t.Measures)!=len(parsed.Song.Headers){return song,fmt.Errorf("track %d has %d measures vs %d headers",index,len(t.Measures),len(parsed.Song.Headers))}
  part:=store.ImportedTrack{Name:strings.TrimSpace(t.Name),Kind:"instrument",Notes:[]store.InputNote{}}
  if part.Name==""{part.Name=fmt.Sprintf("TG Track %d",index+1)}
  isDrums:=channels[t.ChannelID].Bank==128
  if isDrums{part.Kind="drums"}else if len(t.Strings)==0{return song,fmt.Errorf("track %q has no tuning",part.Name)}
  previous:=map[string]int{}
  for measureIndex,m:=range t.Measures {
   h:=parsed.Song.Headers[measureIndex]
   measureLength:=int64(h.Signature.Num)*wholePrecise/int64(h.Signature.Den)
   for _,b:=range m.Beats{
    if b.Start<start||b.Start>=start+measureLength{return song,fmt.Errorf("track %q: beat outside bar %d",part.Name,measureIndex+1)}
    tick:=int64(math.Round(float64(b.Start-wholePrecise/4)*float64(store.PPQ*4)/float64(wholePrecise)))
    for _,v:=range b.Voices{
     if v.Empty=="true"||len(v.Notes)==0{continue}
     if v.Duration.Value<1||v.Duration.Value>128{return song,errors.New("invalid TG note duration")}
     q:=float64(store.PPQ*4)/float64(v.Duration.Value)
     switch v.Duration.Dotted{case "dotted":q*=1.5;case "doubleDotted":q*=1.75}
     if v.Duration.Division.Enters>0{if v.Duration.Division.Times<1{return song,errors.New("invalid TG tuplet")};q*=float64(v.Duration.Division.Times)/float64(v.Duration.Division.Enters)}
     dur:=int64(math.Round(q));if dur<1{return song,errors.New("TG duration rounds to zero")}
     velocity:=95
     for _,n:=range v.Notes {
      if n.Velocity>0{velocity=n.Velocity}
      pitch:=n.Fret+t.Offset
      if !isDrums{
       if n.String<1||n.String>len(t.Strings){return song,fmt.Errorf("invalid TG string %d on %q",n.String,part.Name)}
       pitch+=t.Strings[n.String-1]
      }else{pitch=n.Fret}
      key:=strconv.Itoa(n.String)+":"+strconv.Itoa(pitch)
      if n.Tie {
       prev,ok:=previous[key];if !ok{return song,fmt.Errorf("orphan TG tie at tick %d",tick)}
       old:=&part.Notes[prev];if old.Tick+old.Duration!=tick{return song,fmt.Errorf("noncontiguous TG tie at tick %d",tick)}
       old.Duration+=dur;continue
      }
      input:=store.InputNote{Tick:tick,Duration:dur,Pitch:pitch,Velocity:velocity}
      if err:=store.Validate(input);err!=nil{return song,fmt.Errorf("TG note: %w",err)}
      previous[key]=len(part.Notes);part.Notes=append(part.Notes,input)
     }
    }
   }
   start+=measureLength
  }
  start=wholePrecise/4
  if len(part.Notes)>0{song.Tracks=append(song.Tracks,part)}
 }
 if len(song.Tracks)==0{return song,errors.New("TG contains no notes")}
 return song,nil
}
func IsTG(path string)bool{return strings.EqualFold(strings.TrimPrefix(strings.ToLower(path[strings.LastIndex(path,string(os.PathSeparator))+1:]),"."),"tg") || strings.HasSuffix(strings.ToLower(path),".tg")}
