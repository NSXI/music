package desktop

import("context";"errors";"sync/atomic")
type FileFilter struct{DisplayName,Pattern string}
type OpenDialogOptions struct{Title,DefaultDirectory string;Filters []FileFilter}
var OpenURLHandler func(string)error
var fullscreen atomic.Bool
func OpenDirectoryDialog(context.Context,OpenDialogOptions)(string,error){return "",errors.New("استخدم منتقي المجلدات في أندرويد")}
func OpenFileDialog(context.Context,OpenDialogOptions)(string,error){return "",errors.New("استخدم منتقي الملفات في أندرويد")}
func OpenMultipleFilesDialog(context.Context,OpenDialogOptions)([]string,error){return nil,errors.New("استخدم منتقي الملفات في أندرويد")}
func BrowserOpenURL(_ context.Context,address string)error{if OpenURLHandler!=nil{return OpenURLHandler(address)};return errors.New("واجهة أندرويد غير جاهزة")}
func WindowIsFullscreen(context.Context)bool{return fullscreen.Load()}
func WindowIsMaximised(context.Context)bool{return false}
func WindowFullscreen(context.Context){fullscreen.Store(true)}
func WindowUnfullscreen(context.Context){fullscreen.Store(false)}
func WindowMaximise(context.Context){}
func WindowMinimise(context.Context){}
func WindowToggleMaximise(context.Context){}
func Quit(context.Context){}
