package store

import (
 "testing"
)
func TestVocalPatchAutoCreateAndSafeNoteUpdates(t *testing.T){
 root:=t.TempDir()
 if err:=Init(root,Settings{Name:"Miku",Tempo:184,MeterNum:4,MeterDen:4});err!=nil{t.Fatal(err)}
 s,err:=Open(root);if err!=nil{t.Fatal(err)}
 defer s.Close()
 intro:=VocalPatch{Track:"SideVocal",Singer:"Original",CreateTrack:true,BaseRevision:0,Changes:[]VocalChange{
  {Action:"add",Note:&InputNote{Tick:0,Duration:240,Pitch:65,Velocity:92,Lyric:"っ"}},
  {Action:"add",Note:&InputNote{Tick:240,Duration:240,Pitch:67,Velocity:95,Lyric:"br2"}},
 }}
 first,err:=s.ApplyVocalPatch(intro);if err!=nil{t.Fatal(err)}
 if first.Revision!=1||first.Added!=2||first.Track.Kind!="vocal"{t.Fatalf("unexpected result: %+v",first)}
 ns,err:=s.Notes(first.Track.ID,0,0);if err!=nil{t.Fatal(err)}
 if len(ns)!=2||ns[0].Lyric!="っ"||ns[1].Lyric!="br2"{t.Fatalf("notes lost: %+v",ns)}
 lyric:="きゃ"
 pitch:=70
 second,err:=s.ApplyVocalPatch(VocalPatch{Track:"SideVocal",BaseRevision:first.Revision,Changes:[]VocalChange{
  {Action:"update",NoteID:ns[0].ID,Pitch:&pitch,Lyric:&lyric},
 }});if err!=nil{t.Fatal(err)}
 if second.Updated!=1||second.Revision!=2{t.Fatalf("update not recorded: %+v",second)}
 items,err:=s.Notes(first.Track.ID,0,0);if err!=nil{t.Fatal(err)}
 if len(items)!=2||items[0].ID!=ns[0].ID||items[0].Pitch!=70||items[0].Lyric!="きゃ"||items[1].ID!=ns[1].ID{
  t.Fatalf("vocal patch deleted unrelated notes or regenerated IDs: %+v",items)
 }
 if _,err:=s.ApplyVocalPatch(intro);err==nil{t.Fatal("stale revision accepted")}
 invalid:=VocalPatch{Track:"SideVocal",BaseRevision:second.Revision,Changes:[]VocalChange{
  {Action:"add",Note:&InputNote{Tick:900,Duration:100,Pitch:72,Velocity:99,Lyric:"あ"}},
  {Action:"update",NoteID:"badID",Pitch:&pitch},
 }}
 if _,err:=s.ApplyVocalPatch(invalid);err==nil{t.Fatal("invalid update accepted")}
 after,err:=s.Notes(first.Track.ID,0,0);if err!=nil{t.Fatal(err)}
 if len(after)!=2{t.Fatalf("rollback failed, found %d notes",len(after))}
 revisions,err:=s.Revisions(10);if err!=nil{t.Fatal(err)}
 if len(revisions)!=2||revisions[0].Operation!="vocal.edit"{t.Fatalf("history incorrect: %+v",revisions)}
}
