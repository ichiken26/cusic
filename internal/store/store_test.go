package store

import (
 "testing"
)
func TestProjectPatchAndRevert(t *testing.T){
 root:=t.TempDir()
 if err:=Init(root,Settings{Name:"test",Tempo:168,MeterNum:4,MeterDen:4});err!=nil{t.Fatal(err)}
 s,err:=Open(root);if err!=nil{t.Fatal(err)};defer s.Close()
 bass,r,err:=s.AddTrack("Bass","instrument","");if err!=nil{t.Fatal(err)}
 _,r,err=s.AddNote(bass.ID,InputNote{Tick:0,Duration:480,Pitch:40,Velocity:100});if err!=nil{t.Fatal(err)}
 previous:=r
 revised:=InputNote{Tick:240,Duration:240,Pitch:42,Velocity:96}
 p:=Patch{BaseRevision:previous,Track:bass.ID,FromBar:1,ToBar:1,Message:"New phrase",Notes:[]InputNote{revised}}
 r,err=s.ApplyPatch(p);if err!=nil{t.Fatal(err)}
 notes,err:=s.Notes(bass.ID,0,0);if err!=nil{t.Fatal(err)}
 if len(notes)!=1||notes[0].Tick!=240||notes[0].Pitch!=42{t.Fatalf("wrong notes: %#v",notes)}
 if _,err=s.ApplyPatch(p);err==nil{t.Fatal("stale patch accepted")}
 if _,err=s.Revert(previous);err==nil{t.Fatal("non-HEAD revert accepted")}
 _,err=s.Revert(r);if err!=nil{t.Fatal(err)}
 notes,err=s.Notes(bass.ID,0,0);if err!=nil{t.Fatal(err)}
 if len(notes)!=1||notes[0].Tick!=0||notes[0].Pitch!=40{t.Fatalf("revert failed: %#v",notes)}
}
func TestRefuseCrossBoundaryAndInvalidNote(t *testing.T){
 root:=t.TempDir()
 if err:=Init(root,Settings{Name:"test",Tempo:120,MeterNum:4,MeterDen:4});err!=nil{t.Fatal(err)}
 s,err:=Open(root);if err!=nil{t.Fatal(err)};defer s.Close()
 track,head,err:=s.AddTrack("Teto","vocal","teto_sv2");if err!=nil{t.Fatal(err)}
 _,head,err=s.AddNote(track.ID,InputNote{Tick:1800,Duration:300,Pitch:60,Velocity:100,Lyric:"あ"});if err!=nil{t.Fatal(err)}
 if _,err=s.ApplyPatch(Patch{BaseRevision:head,Track:track.ID,FromBar:2,ToBar:2,Notes:[]InputNote{}});err==nil{t.Fatal("boundary-crossing note was erased")}
 if _,_,err=s.AddNote(track.ID,InputNote{Tick:0,Duration:-1,Pitch:60,Velocity:100});err==nil{t.Fatal("accepted negative duration")}
}
