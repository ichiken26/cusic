// Package audio analyzes PCM/IEEE float WAV stems without loading audio into RAM.
// It handles common Suno Studio 32-bit float 48 kHz multitrack exports.
package audio

import(
 "bufio"
 "crypto/sha256"
 "encoding/binary"
 "encoding/hex"
 "errors"
 "fmt"
 "io"
 "math"
 "os"
)
type Stats struct{
 SampleRate int `json:"sampleRate"`
 Channels int `json:"channels"`
 Samples int64 `json:"samples"`
 Duration float64 `json:"duration"`
 Peak float64 `json:"peak"`
 RMS float64 `json:"rms"`
 SHA256 string `json:"sha256"`
 BandRMS map[string]float64 `json:"bandRms,omitempty"`
}
func Inspect(path string)(Stats,error){
 result:=Stats{}
 f,err:=os.Open(path);if err!=nil{return result,err};defer f.Close()
 info,err:=f.Stat();if err!=nil{return result,err}
 if info.Size()<44{return result,errors.New("not a WAV file")}
 br:=bufio.NewReaderSize(f,1024*1024)
 hdr:=make([]byte,12);if _,err=io.ReadFull(br,hdr);err!=nil{return result,err}
 if string(hdr[:4])!="RIFF"||string(hdr[8:])!="WAVE"{return result,errors.New("only RIFF WAVE supported (RF64 not yet supported)")}
 format,bits:=uint16(0),uint16(0)
 dataOffset,dataSize:=int64(-1),int64(0)
 offset:=int64(12)
 for offset+8<=info.Size(){
  c:=make([]byte,8);if _,err=io.ReadFull(br,c);err!=nil{return result,err}
  offset+=8;length:=int64(binary.LittleEndian.Uint32(c[4:]))
  if length>info.Size()-offset{return result,errors.New("truncated RIFF chunk")}
  switch string(c[:4]){
  case "fmt ":
   if length<16||length>4096{return result,errors.New("invalid fmt chunk")}
   b:=make([]byte,length);if _,err=io.ReadFull(br,b);err!=nil{return result,err}
   format=binary.LittleEndian.Uint16(b[:2]);result.Channels=int(binary.LittleEndian.Uint16(b[2:4]));result.SampleRate=int(binary.LittleEndian.Uint32(b[4:8]));bits=binary.LittleEndian.Uint16(b[14:16])
   if format==0xFFFE && len(b)>=40 {format=binary.LittleEndian.Uint16(b[24:26])}
  case "data":
   dataOffset=offset;dataSize=length
   // Seek over data without decoding the entire file in one allocation.
   if _,err=f.Seek(offset+length+length%2,io.SeekStart);err!=nil{return result,err}
   br.Reset(f);offset+=length+length%2;continue
  default:
   if _,err=io.CopyN(io.Discard,br,length);err!=nil{return result,err}
  }
  offset+=length
  if length%2==1{if _,err=br.ReadByte();err!=nil{return result,err};offset++}
 }
 if dataOffset<0||result.Channels<1||result.Channels>32||result.SampleRate<8000||result.SampleRate>384000{return result,errors.New("unsupported WAV header")}
 if !(format==1&&(bits==16||bits==24||bits==32)||format==3&&(bits==32||bits==64)){return result,fmt.Errorf("unsupported WAV coding: format=%d bits=%d",format,bits)}
 step:=int(bits/8);if dataSize%int64(step*result.Channels)!=0{return result,errors.New("unaligned WAV audio data")}
 result.Samples=dataSize/int64(step*result.Channels)
 result.Duration=float64(result.Samples)/float64(result.SampleRate)
 if _,err=f.Seek(dataOffset,io.SeekStart);err!=nil{return result,err}
 block:=make([]byte,step*result.Channels*4096)
 remaining:=dataSize;sumSquares:=float64(0);total:=int64(0)
 // A simple streaming bank of one-pole low-pass crossovers estimates broad
 // spectrum energy. It is an approximation, NOT a mastering-grade EQ meter.
 boundaries:=[]float64{60,120,250,500,2000,6000,16000}
 alpha:=make([]float64,len(boundaries));lp:=make([]float64,len(boundaries))
 for i,freq:=range boundaries{alpha[i]=1-math.Exp(-2*math.Pi*freq/float64(result.SampleRate))}
 bands:=make([]float64,len(boundaries)+1);frames:=int64(0)
 mono:=float64(0);channelIndex:=0
 for remaining>0{
  size:=int64(len(block));if remaining<size{size=remaining}
  if _,err=io.ReadFull(f,block[:int(size)]);err!=nil{return result,err}
  for i:=0;i<int(size);i+=step{
   var sample float64
   switch {
   case format==3&&bits==32:sample=float64(math.Float32frombits(binary.LittleEndian.Uint32(block[i:])))
   case format==3&&bits==64:sample=math.Float64frombits(binary.LittleEndian.Uint64(block[i:]))
   case format==1&&bits==16:sample=float64(int16(binary.LittleEndian.Uint16(block[i:])))/32768
   case format==1&&bits==24:
    v:=int32(block[i])|int32(block[i+1])<<8|int32(block[i+2])<<16
    if v&0x800000!=0{v|=^int32(0xFFFFFF)}
    sample=float64(v)/8388608
   case format==1&&bits==32:sample=float64(int32(binary.LittleEndian.Uint32(block[i:])))/2147483648
   }
   if math.IsNaN(sample)||math.IsInf(sample,0){return result,errors.New("non-finite WAV sample")}
   if math.Abs(sample)>result.Peak{result.Peak=math.Abs(sample)}
   sumSquares+=sample*sample;total++
   mono+=sample;channelIndex++
   if channelIndex==result.Channels{
    mono/=float64(result.Channels)
    prev:=float64(0)
    for k:=range lp{
     lp[k]+=alpha[k]*(mono-lp[k])
     v:=lp[k]-prev;bands[k]+=v*v;prev=lp[k]
    }
    v:=mono-prev;bands[len(bands)-1]+=v*v
    frames++;mono=0;channelIndex=0
   }
  }
  remaining-=size
 }
 if total>0{result.RMS=math.Sqrt(sumSquares/float64(total))}
 if frames>0{
  keys:=[]string{"sub_0_60_hz","bass_60_120_hz","bass_120_250_hz","lowmid_250_500_hz","mid_500_2000_hz","presence_2000_6000_hz","high_6000_16000_hz","air_above_16000_hz"}
  result.BandRMS=map[string]float64{}
  for i,k:=range keys{result.BandRMS[k]=math.Sqrt(bands[i]/float64(frames))}
 }
 if _,err=f.Seek(0,io.SeekStart);err!=nil{return result,err}
 h:=sha256.New();if _,err=io.Copy(h,f);err!=nil{return result,err}
 result.SHA256=hex.EncodeToString(h.Sum(nil))
 return result,nil
}
