package main

/*
#include <stdlib.h>
*/
import "C"
import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net"
 "net/http"
 "os"
 "os/exec"
 "path/filepath"
 "reflect"
 "strings"
 "sync"
 "time"
 "unsafe"
 "nawa.android/core/desktop"
)
var mobileStudio *Studio
var mobilePaths=map[string]string{}
var mobileEnv []string
var mobileToken,mobileAddress string
type mobileWork struct{ID string `json:"id"`;Kind string `json:"kind"`;Input json.RawMessage `json:"input"`}
type mobileResult struct{Data json.RawMessage;Error string}
var workQueue=make(chan mobileWork,128)
var workMu sync.Mutex
var workWaiting=map[string]chan mobileResult{}
func mobileTask(ctx context.Context,kind string,input []byte)([]byte,error){
 var raw [16]byte;rand.Read(raw[:]);id:=hex.EncodeToString(raw[:]);done:=make(chan mobileResult,1)
 workMu.Lock();workWaiting[id]=done;workMu.Unlock();defer func(){workMu.Lock();delete(workWaiting,id);workMu.Unlock()}()
 select{case workQueue<-mobileWork{id,kind,input}:case <-ctx.Done():return nil,ctx.Err()}
 select{case result:=<-done:if result.Error!=""{return nil,errors.New(result.Error)};return result.Data,nil;case <-ctx.Done():return nil,ctx.Err()}
}
func mobileCommand(ctx context.Context,name string,args ...string)*exec.Cmd {
 if name=="yt-dlp" {args=append([]string{mobilePaths["ytdlpZip"],"--js-runtimes","quickjs:"+mobilePaths["quickjs"],"--ffmpeg-location",mobilePaths["ffmpeg"]},args...);name=mobilePaths["python"]} else if p:=mobilePaths[name];p!="" {name=p}
 cmd:=exec.CommandContext(ctx,name,args...);cmd.Env=withWorkerEnvironment(os.Environ(),mobileEnv);return cmd
}
func mobileJSON(value any)*C.char{b,e:=json.Marshal(value);if e!=nil{b=[]byte(`{"ok":false,"error":"JSON failure"}`)};return C.CString(string(b))}
func mobileError(e error)*C.char{return mobileJSON(map[string]any{"ok":false,"error":e.Error()})}
//export nawa_start
func nawa_start(root,configuration,secret *C.char)*C.char {
 if mobileStudio!=nil{return mobileJSON(map[string]any{"ok":true,"address":mobileAddress,"token":mobileToken})}
 if e:=configureSecret(C.GoString(secret));e!=nil{return mobileError(e)}
 var config struct{Paths map[string]string `json:"paths"`;Env []string `json:"env"`}
 if e:=json.Unmarshal([]byte(C.GoString(configuration)),&config);e!=nil{return mobileError(e)}
 mobilePaths=config.Paths;mobileEnv=config.Env
 s:=NewStudio(C.GoString(root));mobileStudio=s;s.start(context.Background())
 desktop.OpenURLHandler=func(address string)error{raw,_:=json.Marshal(map[string]string{"url":address});ctx,cancel:=context.WithTimeout(context.Background(),2*time.Minute);defer cancel();_,e:=mobileTask(ctx,"openurl",raw);return e}
 listener,e:=net.Listen("tcp","127.0.0.1:0");if e!=nil{return mobileError(e)}
 var token [32]byte;rand.Read(token[:]);mobileToken=hex.EncodeToString(token[:]);mobileAddress="http://"+listener.Addr().String()
 server:=&http.Server{ReadHeaderTimeout:10*time.Second,Handler:http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){if r.Header.Get("X-Nawa-Token")!=mobileToken{http.Error(w,"Forbidden",403);return};s.MediaHandler().ServeHTTP(w,r)})};go server.Serve(listener)
 return mobileJSON(map[string]any{"ok":true,"address":mobileAddress,"token":mobileToken})
}
//export nawa_rpc
func nawa_rpc(method,payload *C.char)(out *C.char){
 defer func(){if recover()!=nil{out=mobileError(errors.New("تعذر تنفيذ طلب التطبيق"))}}()
 if mobileStudio==nil{return mobileError(errors.New("المحرك غير جاهز"))}
 name:=C.GoString(method);var args []json.RawMessage
 if e:=json.Unmarshal([]byte(C.GoString(payload)),&args);e!=nil{return mobileError(e)}
 if name=="GetState"&&len(args)==0{return mobileJSON(map[string]any{"ok":true,"result":mobileStudio.mobileStateJSON()})}
 fn:=reflect.ValueOf(mobileStudio).MethodByName(name);if !fn.IsValid(){return mobileError(fmt.Errorf("Unknown method: %s",name))}
 if fn.Type().NumIn()!=len(args){return mobileError(errors.New("Invalid argument count"))}
 values:=make([]reflect.Value,len(args));for i,raw:=range args{v:=reflect.New(fn.Type().In(i));if e:=json.Unmarshal(raw,v.Interface());e!=nil{return mobileError(e)};values[i]=v.Elem()}
 result:=fn.Call(values);var value any
 for _,v:=range result{if v.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()){if !v.IsNil(){return mobileError(v.Interface().(error))}}else{value=v.Interface()}}
 return mobileJSON(map[string]any{"ok":true,"result":value})
}
//export nawa_poll
func nawa_poll()*C.char{select{case work:=<-workQueue:return mobileJSON(work);case <-time.After(time.Second):return C.CString("null")}}
//export nawa_complete
func nawa_complete(id,data,errorText *C.char){workMu.Lock();done:=workWaiting[C.GoString(id)];workMu.Unlock();if done!=nil{select{case done<-mobileResult{json.RawMessage(C.GoString(data)),C.GoString(errorText)}:default:}}}
//export nawa_free
func nawa_free(value *C.char){C.free(unsafe.Pointer(value))}
func (s *Studio) MobileImport(paths []string)(int,error){return s.importPaths(context.Background(),paths)}
func (s *Studio) MobileProgress(kind string,progress float64,phase string){s.mu.Lock();defer s.mu.Unlock();if kind=="alignment"{s.alignState.Progress=progress;s.alignState.Phase=phase};if kind=="translation"{s.translationState.Progress=progress;s.translationState.Phase=phase}}
func (s *Studio) MobileSummary() map[string]any {s.mu.Lock();defer s.mu.Unlock();pending:=len(s.state.AlignmentQueue)+len(s.state.TranslationQueue);workMu.Lock();pending+=len(workWaiting);workMu.Unlock();for _,j:=range s.state.Jobs{if j.Status=="working"||j.Status=="queued"{pending++}};return map[string]any{"pending":pending,"alignment":s.alignState.Running,"translation":s.translationState.Running,"folder":s.folderImport.Running}}
func (s *Studio) MobileWorkActive(id string)bool{workMu.Lock();defer workMu.Unlock();return workWaiting[id]!=nil}
func (s *Studio) mobileExport(id string){t,ok:=s.task(id);if !ok{return};raw,_:=json.Marshal(map[string]any{"id":id,"paths":[]string{t.Path,t.Cover,t.Lyrics}});ctx,cancel:=context.WithTimeout(context.Background(),30*time.Minute);defer cancel();if _,e:=mobileTask(ctx,"export",raw);e!=nil{s.edit(id,func(t *Task){t.Warning="حُفظ الملف داخل التطبيق؛ تعذر تصديره إلى المجلد المختار / Export failed: "+e.Error()});s.mu.Lock();s.save();s.mu.Unlock()}}
func (s *Studio) mobileStateJSON()json.RawMessage{s.mu.Lock();defer s.mu.Unlock();r:=s.state;r.SpotifyConnected=s.spotifyToken!="";r.MusixmatchConfigured=s.musixmatchKey!="";r.MetadataConfigured=map[string]bool{};for k,v:=range s.metadataKeys{r.MetadataConfigured[k]=v!=""};r.Resource=s.resourceState;r.Resource.Mode=normalizeResourceMode(s.state.Preferences.ResourceMode);r.FolderImport=s.folderImport;r.Alignment=s.alignState;r.Alignment.Ready=s.alignmentReady();r.Alignment.Pending=len(s.state.AlignmentQueue);r.Translation=s.translationState;r.Translation.Ready=s.translationReady();r.Translation.Pending=len(s.state.TranslationQueue);b,_:=json.Marshal(r);return b}
func (s *Studio) MobileCopyState(running bool,current string,processed,total int){s.mu.Lock();defer s.mu.Unlock();s.folderImport=FolderImportState{Running:running,Scanning:running,Current:current,Processed:processed,Total:total,Errors:[]string{}}}
func (s *Studio) GetFonts()[]string{return []string{"sans-serif","serif","monospace","sans-serif-condensed","cursive"}}
func (s *Studio) SetLyricsFullscreen(enabled bool)error{if enabled{desktop.WindowFullscreen(s.ctx)}else{desktop.WindowUnfullscreen(s.ctx)};return nil}
func moveNew(source,destination string)error {out,e:=os.OpenFile(destination,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600);if e!=nil{return e};in,e:=os.Open(source);if e!=nil{out.Close();os.Remove(destination);return e};_,e=io.Copy(out,in);in.Close();if e==nil{e=out.Sync()};closeError:=out.Close();if e==nil{e=closeError};if e!=nil{os.Remove(destination);return e};return os.Remove(source)}
func (s *Studio) MobileAddFolder(path string)error{clean:=filepath.Clean(path);if !strings.HasPrefix(clean,s.root+string(os.PathSeparator)){return errors.New("مجلد غير مصرح")};s.mu.Lock();defer s.mu.Unlock();for _,old:=range s.state.Preferences.LibraryFolders{if old==clean{return nil}};s.state.Preferences.LibraryFolders=append(s.state.Preferences.LibraryFolders,clean);s.save();return nil}
func main(){}
