package store
import("testing")
func TestSyncPreservesVocalsAndRevertsNotes(t *testing.T){
 dir:=t.TempDir();if err:=Init(dir,Settings{Name:"test",Tempo:120,MeterNum:4,MeterDen:4});err!=nil{t.Fatal(err)}
 s,err:=Open(dir);if err!=nil{t.Fatal(err)};defer s.Close()
 b,_,err:=s.AddTrack("Bass","instrument","");if err!=nil{t.Fatal(err)}
 v,_,err:=s.AddTrack("Miku","vocal","miku_v4x");if err!=nil{t.Fatal(err)}
 _,_,err=s.AddNote(b.ID,InputNote{Tick:0,Duration:480,Pitch:40,Velocity:100});if err!=nil{t.Fatal(err)}
 _,_,err=s.AddNote(v.ID,InputNote{Tick:0,Duration:480,Pitch:69,Velocity:100,Lyric:"あ"});if err!=nil{t.Fatal(err)}
 imports:=[]ImportedTrack{
  {Name:"Bass",Notes:[]InputNote{{Tick:240,Duration:480,Pitch:43,Velocity:100}}},
  {Name:"Miku",Notes:[]InputNote{{Tick:240,Duration:480,Pitch:70,Velocity:100}}},
 }
 plan,err:=s.PreviewSync(imports);if err!=nil{t.Fatal(err)}
 if !plan.Tracks[0].Changed||!plan.Tracks[1].Skipped{t.Fatalf("unsafe plan: %#v",plan)}
 rev,err:=s.SyncTracks(imports,plan.BaseRevision);if err!=nil{t.Fatal(err)}
 bass,err:=s.Notes(b.ID,0,0);if err!=nil||bass[0].Pitch!=43{t.Fatalf("sync failed: %#v %v",bass,err)}
 vocals,err:=s.Notes(v.ID,0,0);if err!=nil||vocals[0].Lyric!="あ"||vocals[0].Pitch!=69{t.Fatalf("vocal changed: %#v %v",vocals,err)}
 if _,err=s.SyncTracks(imports,plan.BaseRevision);err==nil{t.Fatal("accepted stale sync")}
 if _,err=s.Revert(rev);err!=nil{t.Fatal(err)}
 bass,err=s.Notes(b.ID,0,0);if err!=nil||bass[0].Pitch!=40{t.Fatalf("sync not reversible: %#v %v",bass,err)}
}
