package main

// An opt-in, isolated documentation harness. It never launches Wails, opens the
// user's library, uses accounts, or plays sound outside a muted browser page.
import (
 "context"
 "encoding/binary"
 "encoding/json"
 "fmt"
 "image"
 "image/color"
 "image/png"
 "net"
 "net/http"
 "os"
 "path/filepath"
 "reflect"
 "strings"
 "testing"
 "time"
)

func TestDocumentationPreviewServer(t *testing.T){
 location:=os.Getenv("NAWA_DOCS_PREVIEW_ROOT");if location==""{t.Skip("opt-in local documentation harness")}
 root:=filepath.Join(location,"library-preview");os.MkdirAll(root,0700)
 s:=NewStudio(root);s.state.Jobs=[]Task{};s.state.Preferences.AutoTranslate=false;s.state.Preferences.AutoAlign=false;s.state.Preferences.AutoMatch=false;s.ctx=context.Background()
 titles:=[]string{"First light","Blue hour","صوتٌ قريب"};genres:=[]string{"Ambient","Electronic","Instrumental"}
 hues:=[]color.RGBA{{R:27,G:84,B:116,A:255},{R:61,G:47,B:107,A:255},{R:126,G:74,B:59,A:255}}
 for i,title:=range titles{
  id:=fmt.Sprintf("nawa-doc-%d",i+1);audio:=filepath.Join(root,id+".wav");frames:=96000*12;data:=make([]byte,44+frames*6);copy(data,"RIFF");binary.LittleEndian.PutUint32(data[4:],uint32(len(data)-8));copy(data[8:],"WAVEfmt ");binary.LittleEndian.PutUint32(data[16:],16);binary.LittleEndian.PutUint16(data[20:],1);binary.LittleEndian.PutUint16(data[22:],2);binary.LittleEndian.PutUint32(data[24:],96000);binary.LittleEndian.PutUint32(data[28:],576000);binary.LittleEndian.PutUint16(data[32:],6);binary.LittleEndian.PutUint16(data[34:],24);copy(data[36:],"data");binary.LittleEndian.PutUint32(data[40:],uint32(len(data)-44));os.WriteFile(audio,data,0600)
  picture:=image.NewRGBA(image.Rect(0,0,640,640));base:=hues[i]
  for y:=0;y<640;y++{for x:=0;x<640;x++{v:=float64(x+y)/1280;rr,gg,bb:=float64(base.R)*(0.5+v),float64(base.G)*(0.5+v),float64(base.B)*(0.5+v);dx,dy:=x-320,y-320;d:=dx*dx+dy*dy;if d<45000&&d>41000||d<15000&&d>13000||d<1800 {rr+=60;gg+=80;bb+=80};picture.SetRGBA(x,y,color.RGBA{uint8(min(255,int(rr))),uint8(min(255,int(gg))),uint8(min(255,int(bb))),255})}}
  cover:=filepath.Join(root,id+".png");f,_:=os.Create(cover);png.Encode(f,picture);f.Close()
  song:=Song{ID:id,Title:title,Artist:"Nawa Studio",Album:"Original preview",Provider:"local",Duration:12,Genres:[]string{genres[i]},Artwork:"/art/"+id}
  s.known[id]=song;s.state.Jobs=append(s.state.Jobs,Task{ID:id,Song:song,Path:audio,Cover:cover,Local:true,Status:"done",GenreChecked:true,Source:"local",Quality:Quality{Codec:"PCM_S24LE",Bits:24,Rate:96000,Channels:2,Bitrate:4608000,Lossless:true,Duration:12},Log:[]string{}})
 }
 s.SetUpdateFeed("")
 lrc:="[00:00.00]Music brings us together\n[00:03.00]We follow every sound\n[00:06.00]A quiet place to listen\n[00:09.00]We find our way home"
 for _,task:=range s.state.Jobs{
  if e:=s.SaveLyricsText(task.ID,lrc,true);e!=nil{t.Fatal(e)}
  v,e:=s.GetTranslation(task.ID,"ar","en",false);if e!=nil{t.Fatal(e)}
  if e=s.SaveTranslation(task.ID,"ar","en",v.OriginalHash,[]string{"الموسيقى تجمعنا معًا","نتبع كل صوت","مكان هادئ للاستماع","نجد طريقنا إلى البيت"});e!=nil{t.Fatal(e)}
 }
 listener,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{t.Fatal(e)}
 done:=make(chan struct{});mux:=http.NewServeMux();server:=&http.Server{Handler:mux,ReadHeaderTimeout:10*time.Second}
 mux.HandleFunc("/rpc/",func(w http.ResponseWriter,r *http.Request){
  w.Header().Set("Content-Type","application/json");name:=strings.TrimPrefix(r.URL.Path,"/rpc/");var args []json.RawMessage;json.NewDecoder(http.MaxBytesReader(w,r.Body,2<<20)).Decode(&args)
  output:=map[string]any{"ok":true}
  if name=="SetLyricsFullscreen"||name=="Window"{output["result"]=true;json.NewEncoder(w).Encode(output);return}
  if name=="Search"{tracks:=[]Song{};for _,job:=range s.GetState().Jobs{tracks=append(tracks,job.Song)};output["result"]=map[string]any{"title":"Nawa Studio · Local preview","tracks":tracks,"complete":true,"total":len(tracks),"providerCounts":map[string]int{"local":len(tracks)},"notice":"بيانات عرض محلية أصلية / Original local preview data"};json.NewEncoder(w).Encode(output);return}
  method:=reflect.ValueOf(s).MethodByName(name)
  if !method.IsValid()||method.Type().NumIn()!=len(args){json.NewEncoder(w).Encode(map[string]any{"ok":false,"error":"Unsupported preview request: "+name});return}
  values:=make([]reflect.Value,len(args));for i,arg:=range args{p:=reflect.New(method.Type().In(i));if e:=json.Unmarshal(arg,p.Interface());e!=nil{json.NewEncoder(w).Encode(map[string]any{"ok":false,"error":e.Error()});return};values[i]=p.Elem()}
  for _,v:=range method.Call(values){if v.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()){if !v.IsNil(){output["ok"]=false;output["error"]=v.Interface().(error).Error()}}else{output["result"]=v.Interface()}}
  json.NewEncoder(w).Encode(output)
 })
 for _,prefix:=range []string{"/media/","/art/","/decoded/","/artwork/"}{mux.Handle(prefix,s.MediaHandler())}
 mux.HandleFunc("/__stop",func(w http.ResponseWriter,r *http.Request){if r.Method=="POST"{select{case <-done:default:close(done)}};w.WriteHeader(200)})
 mux.Handle("/",http.FileServer(http.Dir("frontend")))
 jsonData,_:=json.Marshal(map[string]string{"url":"http://"+listener.Addr().String(),"scope":"isolated original local preview; not user data"});os.WriteFile(filepath.Join(location,"server.json"),jsonData,0600)
 go server.Serve(listener);select{case <-done:case <-time.After(20*time.Minute):t.Error("preview timed out")};ctx,cancel:=context.WithTimeout(context.Background(),time.Second);defer cancel();server.Shutdown(ctx)
}
