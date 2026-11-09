package analysis

import (
 "context"
 "encoding/binary"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
)

func TestValidateRequest(t *testing.T){
 if _,err:=Analyze(context.Background(),nil,10);err==nil{t.Fatal("accepted no files")}
 if _,err:=Analyze(context.Background(),[]File{{Name:"Bass",Path:"bass.wav"}},2049);err==nil{t.Fatal("accepted too many windows")}
}
func TestPythonWaveIntegration(t *testing.T){
 python,pre,err:=Python();if err!=nil{t.Skip(err)}
 check:=exec.Command(python,append(pre,"-c","import numpy, scipy")...)
 if err:=check.Run();err!=nil{t.Skip("NumPy/SciPy not installed; run the dedicated Python CI job")}
 filename:=filepath.Join(t.TempDir(),"stem.wav")
 f,err:=os.Create(filename);if err!=nil{t.Fatal(err)}
 // 0.5 seconds of low-frequency PCM16 at 8 kHz.
 data:=make([]byte,8000)
 for i:=0;i<4000;i++{
  val:=int16(14000)
  if (i/50)%2==0{val = -val}
  binary.LittleEndian.PutUint16(data[i*2:],uint16(val))
 }
 f.Write([]byte("RIFF"));binary.Write(f,binary.LittleEndian,uint32(36+len(data)))
 f.Write([]byte("WAVEfmt "));binary.Write(f,binary.LittleEndian,uint32(16))
 binary.Write(f,binary.LittleEndian,uint16(1));binary.Write(f,binary.LittleEndian,uint16(1))
 binary.Write(f,binary.LittleEndian,uint32(8000));binary.Write(f,binary.LittleEndian,uint32(16000))
 binary.Write(f,binary.LittleEndian,uint16(2));binary.Write(f,binary.LittleEndian,uint16(16))
 f.Write([]byte("data"));binary.Write(f,binary.LittleEndian,uint32(len(data)));f.Write(data)
 if err=f.Close();err!=nil{t.Fatal(err)}
 report,err:=Analyze(context.Background(),[]File{{Name:"Test Bass",Path:filename}},16)
 if err!=nil{t.Fatal(err)}
 if len(report.Stems)!=1||report.Engine!="numpy-scipy"||report.Stems[0].BandRmsDBFS==nil{
  t.Fatalf("incomplete Python worker output: %+v",report)
 }
}
