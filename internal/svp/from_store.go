package svp

import (
 "errors"
 "fmt"
 "math"

 "github.com/ichiken26/cusic/internal/store"
)

// PayloadFromStore lets cusic create and apply a fresh Teto track directly
// from the song database: no intermediate .json export or SynthV JS is needed.
func PayloadFromStore(s *store.Store,source []byte,sourceTrack,vocalTrack string)(Payload,error){
 var p Payload
 current,err:=s.Track(vocalTrack);if err!=nil{return p,err}
 if current.Kind!="vocal"{return p,fmt.Errorf("cusic track %q is not vocal",vocalTrack)}
 notes,err:=s.Notes(current.ID,0,0);if err!=nil{return p,err}
 if len(notes)==0{return p,errors.New("no edited vocal notes to apply")}
 originalEnd,err:=MaxTick(source,sourceTrack);if err!=nil{return p,err}
 last:=originalEnd
 result:=make([]Note,0,len(notes))
 for _,n:=range notes{
  if n.Tick<0||n.Duration<1||n.Tick>math.MaxInt64-n.Duration{return p,errors.New("note time out of range")}
  if n.Tick+n.Duration>last{last=n.Tick+n.Duration}
  result=append(result,Note{ID:n.ID,TrackID:current.ID,Tick:n.Tick,Duration:n.Duration,
   Pitch:n.Pitch,Velocity:n.Velocity,Lyric:n.Lyric})
 }
 if last<1{return p,errors.New("no source or target note span")}
 p=Payload{Version:1,TrackID:current.ID,TrackName:current.Name,PPQ:store.PPQ,
  FromTick:0,ToTick:last,Notes:result}
 return p,nil
}
