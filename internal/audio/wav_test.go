package audio
import("encoding/binary";"os";"path/filepath";"testing";"math")
func TestInspect16BitWAV(t *testing.T){
 p:=filepath.Join(t.TempDir(),"stem.wav");f,e:=os.Create(p);if e!=nil{t.Fatal(e)}
 data:=[]byte{0,0,0xFF,0x7F,0,0,0,0x80}
 f.Write([]byte("RIFF"));binary.Write(f,binary.LittleEndian,uint32(36+len(data)))
 f.Write([]byte("WAVEfmt "));binary.Write(f,binary.LittleEndian,uint32(16))
 binary.Write(f,binary.LittleEndian,uint16(1));binary.Write(f,binary.LittleEndian,uint16(1))
 binary.Write(f,binary.LittleEndian,uint32(8000));binary.Write(f,binary.LittleEndian,uint32(16000))
 binary.Write(f,binary.LittleEndian,uint16(2));binary.Write(f,binary.LittleEndian,uint16(16))
 f.Write([]byte("data"));binary.Write(f,binary.LittleEndian,uint32(len(data)));f.Write(data);f.Close()
 s,e:=Inspect(p);if e!=nil{t.Fatal(e)}
 if s.SampleRate!=8000||s.Channels!=1||s.Samples!=4||math.Abs(s.Peak-1)>0.001||len(s.SHA256)!=64{t.Fatalf("bad WAV: %#v",s)}
}
