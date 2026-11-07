package main
import (
 "archive/zip"
 "encoding/binary"
 "os"
 "path/filepath"
 "testing"
 "github.com/ichiken26/cusic/internal/dawproject"
)
func TestCLIExchange(t *testing.T){
 root:=t.TempDir()
 old,err:=os.Getwd();if err!=nil{t.Fatal(err)}
 if err=os.Chdir(root);err!=nil{t.Fatal(err)};defer os.Chdir(old)
 for _,args:=range [][]string{
  {"init","--name","Test","--tempo","168"},
  {"track","add","--name","Bass"},
  {"note","add","--track","Bass","--tick","1920","--pitch","40"},
  {"export","--format","midi","--out","bass.mid"},
  {"export","--format","dawproject","--out","bass.dawproject"},
  {"sync","plan","--from","bass.dawproject"},
 }{if err:=run(args);err!=nil{t.Fatalf("%v: %v",args,err)}}
 if _,err:=os.Stat("bass.mid");err!=nil{t.Fatal(err)}
 parsed,err:=dawproject.ReadFile("bass.dawproject");if err!=nil{t.Fatal(err)}
 if len(parsed.Song.Tracks)!=1||parsed.Song.Tracks[0].Notes[0].Pitch!=40{t.Fatalf("%#v",parsed)}
 // Suno stem import and DAWproject embedding.
 os.Mkdir("stems",0755);f,err:=os.Create(filepath.Join("stems","bass.wav"));if err!=nil{t.Fatal(err)}
 data:=[]byte{0,0,0,0,0xff,0x7f,0,0}
 f.Write([]byte("RIFF"));binary.Write(f,binary.LittleEndian,uint32(36+len(data)))
 f.Write([]byte("WAVEfmt "));binary.Write(f,binary.LittleEndian,uint32(16))
 binary.Write(f,binary.LittleEndian,uint16(1));binary.Write(f,binary.LittleEndian,uint16(1))
 binary.Write(f,binary.LittleEndian,uint32(48000));binary.Write(f,binary.LittleEndian,uint32(96000))
 binary.Write(f,binary.LittleEndian,uint16(2));binary.Write(f,binary.LittleEndian,uint16(16))
 f.Write([]byte("data"));binary.Write(f,binary.LittleEndian,uint32(len(data)));f.Write(data);f.Close()
 if err:=run([]string{"suno","import","stems"});err!=nil{t.Fatal(err)}
 if err:=run([]string{"export","--format","dawproject","--out","with-audio.dawproject"});err!=nil{t.Fatal(err)}
 archive,err:=zip.OpenReader("with-audio.dawproject");if err!=nil{t.Fatal(err)};defer archive.Close()
 audioFound:=false;for _,f:=range archive.File{if len(f.Name)>6&&f.Name[:6]=="audio/"{audioFound=true}}
 if !audioFound{t.Fatal("embedded audio missing")}
 if err:=run([]string{"export","--format","dawproject","--out","with-audio.dawproject"});err==nil{t.Fatal("overwrote output")}
}
