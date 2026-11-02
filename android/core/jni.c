#include <jni.h>
#include <stdlib.h>
#include <string.h>
extern char* nawa_start(char*,char*,char*);
extern char* nawa_rpc(char*,char*);
extern char* nawa_poll(void);
extern void nawa_complete(char*,char*,char*);
extern void nawa_free(char*);
static char* input(JNIEnv* e,jbyteArray bytes){jsize n=(*e)->GetArrayLength(e,bytes);char* s=malloc(n+1);(*e)->GetByteArrayRegion(e,bytes,0,n,(jbyte*)s);s[n]=0;return s;}
static jbyteArray output(JNIEnv* e,char* text){jsize n=strlen(text);jbyteArray b=(*e)->NewByteArray(e,n);(*e)->SetByteArrayRegion(e,b,0,n,(jbyte*)text);nawa_free(text);return b;}
JNIEXPORT jbyteArray JNICALL Java_dev_nawa_music_CoreBridge_nativeStart(JNIEnv* e,jobject o,jbyteArray root,jbyteArray config,jbyteArray secret){char *r=input(e,root),*c=input(e,config),*s=input(e,secret);char* out=nawa_start(r,c,s);free(r);free(c);free(s);return output(e,out);}
JNIEXPORT jbyteArray JNICALL Java_dev_nawa_music_CoreBridge_nativeRpc(JNIEnv* e,jobject o,jbyteArray method,jbyteArray args){char *m=input(e,method),*a=input(e,args);char* out=nawa_rpc(m,a);free(m);free(a);return output(e,out);}
JNIEXPORT jbyteArray JNICALL Java_dev_nawa_music_CoreBridge_nativePoll(JNIEnv* e,jobject o){return output(e,nawa_poll());}
JNIEXPORT void JNICALL Java_dev_nawa_music_CoreBridge_nativeComplete(JNIEnv* e,jobject o,jbyteArray id,jbyteArray data,jbyteArray error){char *i=input(e,id),*d=input(e,data),*x=input(e,error);nawa_complete(i,d,x);free(i);free(d);free(x);}
