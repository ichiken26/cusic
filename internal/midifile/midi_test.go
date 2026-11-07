package midifile

import (
 "bytes"
 "testing"
 "github.com/ichiken26/cusic/internal/store"
)
func TestRoundTrip(t *testing.T){
 s:=store.Settings{Name:"demo",Tempo:168,MeterNum:4,MeterDen:4,PPQ:480}
 tr:=store.Track{ID:"b",Name:"Bass",Kind:"instrument"}
 note:=store.Note{ID:"n",TrackID:"b",Tick:1920,Duration:480,Pitch:40,Velocity:97}
 b,e:=Write(s,[]store.Track{tr},map[string][]store.Note{"b":{note}})
 if e!=nil{t.Fatal(e)}
 got,e:=Read(b);if e!=nil{t.Fatal(e)}
 if got.Tempo!=168||got.MeterNum!=4||len(got.Tracks)!=1||len(got.Tracks[0].Notes)!=1{t.Fatalf("bad roundtrip: %#v",got)}
 n:=got.Tracks[0].Notes[0];if n.Tick!=1920||n.Duration!=480||n.Pitch!=40||n.Velocity!=97{t.Fatalf("bad note: %#v",n)}
 if !bytes.Equal(b[:4],[]byte("MThd")){t.Fatal("missing header")}
}
func TestRejectGarbage(t *testing.T){if _,e:=Read([]byte("not a midi"));e==nil{t.Fatal("expected error")}}
func TestVLQ(t *testing.T){
 for _,n:=range []int64{0,127,128,480,1920,0x0FFFFFFF}{
  data:=encodeVLQ(n);p:=0;value,e:=readVLQ(data,&p)
  if e!=nil||value!=n||p!=len(data){t.Fatalf("%d => %x => %d (%v)",n,data,value,e)}
 }
}
