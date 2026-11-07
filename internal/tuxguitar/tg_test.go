package tuxguitar
import(
 "archive/zip"
 "os"
 "path/filepath"
 "testing"
)
func TestReadTG2(t *testing.T){
 p:=filepath.Join(t.TempDir(),"example.tg");f,e:=os.Create(p);if e!=nil{t.Fatal(e)}
 z:=zip.NewWriter(f)
 a,_:=z.Create("version.txt");a.Write([]byte("TuxGuitar_file_format 2.0"))
 b,_:=z.Create("content.xml")
 b.Write([]byte(`<?xml version="1.0"?><TuxGuitarFile><TGVersion major="2" minor="0" revision="0"/><TGSong><TGChannel><id>1</id><bank>0</bank></TGChannel><TGMeasureHeader><timeSignature numerator="4" denominator="4"/><tempo>168</tempo></TGMeasureHeader><TGTrack><name>Bass</name><channelId>1</channelId><TGString>43</TGString><TGString>38</TGString><TGString>33</TGString><TGString>28</TGString><TGMeasure><TGBeat><preciseStart>1441440</preciseStart><voice><duration value="4"/><note value="3" string="4" velocity="98"/></voice></TGBeat></TGMeasure></TGTrack></TGSong></TuxGuitarFile>`))
 z.Close();f.Close()
 song,e:=ReadFile(p);if e!=nil{t.Fatal(e)}
 if song.Tempo!=168||len(song.Tracks)!=1||len(song.Tracks[0].Notes)!=1{t.Fatalf("bad TG: %#v",song)}
 n:=song.Tracks[0].Notes[0];if n.Tick!=0||n.Pitch!=31||n.Duration!=480{t.Fatalf("bad note: %#v",n)}
}
