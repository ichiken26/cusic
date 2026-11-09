package main

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/ichiken26/cusic/internal/store"
)

func TestTempoPendingUntilConfirmed(t *testing.T) {
    root:=t.TempDir()
    if err:=run([]string{"init","--name","Suno Song",root});err!=nil{t.Fatal(err)}
    s,err:=store.Open(root);if err!=nil{t.Fatal(err)}
    settings,err:=s.Settings();if err!=nil{t.Fatal(err)}
    if settings.Tempo!=0{t.Fatalf("expected pending tempo, got %d",settings.Tempo)}
    s.Close()

    previous,err:=os.Getwd();if err!=nil{t.Fatal(err)}
    if err=os.Chdir(root);err!=nil{t.Fatal(err)}
    defer os.Chdir(previous)

    if err:=run([]string{"export","--format","dawproject","--out","bad.dawproject"});err==nil{t.Fatal("export accepted unconfirmed BPM")}
    if _,err:=os.Stat("bad.dawproject");!os.IsNotExist(err){t.Fatal("export touched output before tempo confirmation")}
    if err:=run([]string{"tempo","set","--bpm","184"});err!=nil{t.Fatal(err)}
    s,err=store.Open(root);if err!=nil{t.Fatal(err)}
    settings,err=s.Settings();if err!=nil{t.Fatal(err)}
    if settings.Tempo!=184{t.Fatalf("expected 184, got %d",settings.Tempo)}
    rev,err:=s.Revisions(1);if err!=nil{t.Fatal(err)}
    if len(rev)!=1||rev[0].Operation!="tempo.confirm"{t.Fatalf("missing settings revision: %+v",rev)}
    s.Close()
    if err:=run([]string{"tempo","set","--bpm","120"});err==nil{t.Fatal("re-timed existing project unexpectedly")}
    if err:=run([]string{"export","--format","dawproject","--out","song.dawproject"});err!=nil{t.Fatal(err)}
    if _,err:=os.Stat(filepath.Join(root,"song.dawproject"));err!=nil{t.Fatal(err)}
}

func TestTimeMapAnchors(t *testing.T) {
    beats:=make([]float64,32)
    for i:=range beats{beats[i]=0.2+float64(i)*60.0/184.0}
    mapping,err:=makeTimeMap(beats,12,48000,184,1)
    if err!=nil{t.Fatal(err)}
    if len(mapping)<20{t.Fatal("empty time map")}
    bad:=append([]float64(nil),beats...)
    bad[14]=bad[13] - 0.1
    if _,err=makeTimeMap(bad,12,48000,184,1);err==nil{t.Fatal("accepted non-monotonic anchors")}
}
