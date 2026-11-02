#include <jni.h>
#include <whisper.h>
#include <fstream>
#include <sstream>
#include <vector>
#include <string>
#include <iomanip>
#include <atomic>
static std::atomic<bool> cancelled{false};
extern "C" JNIEXPORT void JNICALL Java_dev_nawa_music_TimingEngine_nativeCancel(JNIEnv*,jobject){cancelled.store(true);}
static std::string utf(JNIEnv* e,jbyteArray a){ auto n=e->GetArrayLength(a); std::string s(n,'\0'); e->GetByteArrayRegion(a,0,n,(jbyte*)s.data()); return s; }
static jbyteArray result(JNIEnv* e,const std::string& s){auto a=e->NewByteArray(s.size()); e->SetByteArrayRegion(a,0,s.size(),(const jbyte*)s.data());return a;}
extern "C" JNIEXPORT jbyteArray JNICALL Java_dev_nawa_music_TimingEngine_nativeTokens(JNIEnv* e,jobject receiver,jbyteArray model,jbyteArray pcm,jbyteArray language,jbyteArray prompt,jint threads){
    cancelled.store(false);
    auto mp=utf(e,model),pp=utf(e,pcm),lang=utf(e,language),pr=utf(e,prompt);
    std::ifstream f(pp,std::ios::binary|std::ios::ate); if(!f) return result(e,"{\"error\":\"Cannot read decoded audio\"}");
    auto bytes=f.tellg(); if(bytes<=0 || bytes>16000LL*4*60*30) return result(e,"{\"error\":\"Alignment supports audio up to 30 minutes\"}");
    std::vector<float> samples((size_t)bytes/4);f.seekg(0);f.read((char*)samples.data(),samples.size()*4);
    auto cp=whisper_context_default_params();cp.use_gpu=false;cp.flash_attn=false;cp.dtw_token_timestamps=true;cp.dtw_aheads_preset=WHISPER_AHEADS_BASE;cp.dtw_mem_size=128*1024*1024;
    auto ctx=whisper_init_from_file_with_params(mp.c_str(),cp);if(!ctx)return result(e,"{\"error\":\"Cannot load Whisper model\"}");
    auto p=whisper_full_default_params(WHISPER_SAMPLING_GREEDY);
    p.n_threads=threads;p.language=lang=="auto"?nullptr:lang.c_str();p.detect_language=false;
    p.translate=false;p.token_timestamps=true;p.split_on_word=true;p.print_progress=false;p.print_realtime=false;p.print_timestamps=false;p.print_special=false;
    p.initial_prompt=pr.c_str();p.suppress_blank=true;p.no_context=false;
    p.abort_callback=[](void*){return cancelled.load();};
    struct Progress {JNIEnv* env; jobject object; jmethodID method;};
    Progress progress{e,receiver,e->GetMethodID(e->GetObjectClass(receiver),"onProgress","(I)V")};
    p.progress_callback_user_data=&progress;
    p.progress_callback=[](whisper_context*,whisper_state*,int value,void* raw){auto* c=(Progress*)raw;if(c->method)c->env->CallVoidMethod(c->object,c->method,value);};
    if(whisper_full(ctx,p,samples.data(),samples.size())!=0){whisper_free(ctx);return result(e,"{\"error\":\"Speech alignment failed\"}");}
    std::ostringstream o;o<<"{\"duration\":"<<samples.size()/16000.0<<",\"mode\":\"attention-dtw\",\"language\":\""<<whisper_lang_str(whisper_full_lang_id(ctx))<<"\",\"tokens\":[";bool first=true;
    for(int s=0;s<whisper_full_n_segments(ctx);s++)for(int i=0;i<whisper_full_n_tokens(ctx,s);i++){
        auto d=whisper_full_get_token_data(ctx,s,i);if(d.id>=whisper_token_eot(ctx))continue;
        const auto* txt=whisper_full_get_token_text(ctx,s,i);std::string hex;const char* digits="0123456789abcdef";
        for(auto q=(const unsigned char*)txt;*q;q++){hex+=digits[*q>>4];hex+=digits[*q&15];}
        if(!first)o<<",";first=false;o<<"{\"hex\":\""<<hex<<"\",\"start\":"<<d.t0/100.0<<",\"end\":"<<d.t1/100.0<<",\"center\":"<<d.t_dtw/100.0<<",\"probability\":"<<d.p<<"}";
    }
    o<<"]}";whisper_free(ctx);return result(e,o.str());
}
