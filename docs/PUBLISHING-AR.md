# بناء ونشر إصدار للجهازين

المجلد `desktop` هو مصدر Windows، و`android` منفذ مستقل يستخدم الواجهة والمنطق المشترك نفسه. مزامنة ثلاثية تحافظ على تغييرات الجوال وتوقف البناء عند تعارضها. لا تُنقل مكتبة المستخدم أو الأدوات التنفيذية من Windows إلى APK.

## المتطلبات

Windows x64، Go 1.26، Python 3.11+، JDK 17، Android SDK 35 وBuild Tools 35.0.0 وNDK 28.2.13676358 وCMake 3.22.1. يشمل المصدر Gradle wrapper 8.11.1. اضبط `ANDROID_HOME` و`JAVA_HOME`، ويمكن تحديد Python بواسطة `NAWA_PYTHON`.

من جذر المستودع:

```powershell
python android/scripts/sync_desktop.py --apply
python android/scripts/test_shared_sync.py
go test -C desktop ./...
go build -C desktop -tags 'desktop,production' -ldflags '-s -w -H windowsgui' -o ../dist/Nawa.exe .
./android/build-core.ps1 -Abi arm64-v8a
./android/gradlew.bat -p android -PnawaAbis=arm64-v8a :app:assembleDebug :app:lintDebug
```

ملف Debug مستقل باسم حزمة ينتهي بـ `.debug`؛ لا يحدّث النسخة المنشورة. لبناء إصدار التحديث، اضبط `NAWA_SIGNING_STORE` لمسار PKCS12 و`NAWA_SIGNING_PASSWORD` في بيئة العملية فقط، بالمفتاح alias=`nawa`، ثم ابنِ `:app:assembleRelease :app:lintRelease`. **احتفظ بنسخة احتياطية خاصة من مفتاح الإصدار نفسه؛ تغييره يمنع ترقية النسخ الموجودة. لا ترفعه للمستودع.**

يتطلب تشغيل Windows مجلد `tools` من الحزمة الأساسية، وفيه FFmpeg وFFprobe وyt-dlp وNode وإشعارات التراخيص. محركا مطابقة التوقيت والترجمة ونماذجهما حزمتان اختياريتان في2.0؛ لا تأتي تلك الملفات الضخمة مع المصدر. يمكن تنزيلهما من إعدادات التطبيق أو فكهما داخل مجلد EXE مع الحفاظ على مسارات `tools/...`. شغّل نسخة الاختبار في مجلد معزول دون بيانات مستخدم.

## نقل التحديثات

1. حدّث `desktop/release.json` و`desktop/release_identity.go` بإصدار ورقم build أكبر. زامن المصدر إلى Android، وعالج أي تعارض أو تغيير يحتاج محولًا أصليًا.
2. ابنِ Windows وAndroid واختبرهما. سجّل إثبات APK بواسطة `android/scripts/build_identity.py --record-release <apk>` بعد التحقق من توقيعه. يحتفظ الإثبات ببصمة المصدر وبصمة APK.
3. جهّز ZIP أساسيًا نظيفًا لويندوز: EXE وأدوات الصوت والمصادر والتراخيص، دون `data` أو التنزيلات أو بيانات الاعتماد أو النماذج الكبيرة. جهّز APK الموقّع وحزمة مصدر نظيفة. ضع محرك التوقيت ونموذجه في `Nawa-Windows-Alignment.zip`، ومحرك الترجمة ونموذجه في `Nawa-Windows-Translation.zip`، مع تراخيص الاعتماد والنماذج داخل كل حزمة. استخدم الحجم وSHA-256 الفعليين لكل ملف.
4. أنشئ إصدار GitHub مسودة وارفع الأصول السبعة: ZIP الأساسي وAPK الموقّع وحزمتي المحركات الاختيارية وZIP المصدر و`nawa-update.json` و`NAWA-SHA256SUMS.txt`. راجع اكتمال كل أصل وحجمه وبصمته على الخادم قبل نشر المسودة.
5. ملف JSON بالشكل الموجود في أصل الإصدار المنشور، schema=1 وappId=`dev.nawa.music`، ورقم الإصدار و`sharedRevision` نفسه، وأصول `windows-x64` و`android-arm64` بروابط HTTPS وحجم وبصمة SHA-256. أضف خريطة `components` بمفتاحي `windows-alignment` و`windows-translation`؛ لكل منهما رابط HTTPS وحجم وSHA-256، ومن الإصدار نفسه. لا تُقدّم منصة واحدة كتحديث مشترك مكتمل.
6. بعد النشر تحقق من الرابط العام أدناه. التطبيق لا يعرض تحديثًا إذا كان build مساويًا للمثبت أو أقدم منه.

[ملف التحديث العام](https://github.com/NSXI/music/releases/latest/download/nawa-update.json)

مسار الفحص في GitHub Actions يتحقق من المصدر ويبني Windows وAPK Debug لكل دفع. نواتجه للاختبار، ولا ينشر APK بتوقيع الإصدار أو يضع مفتاح المطور على GitHub. النشر العام يتطلب الحزم السبع المتحقّق منها، وشهادة Android نفسها، وفحوص المصدر والواجهات والصوت والتوقيت والنسخ الاحتياطي. Windows لا يحمل شهادة Authenticode ما لم توفر شهادة؛ بصمة SHA-256 ليست توقيعًا من جهة تصديق. فحوص المحاكي الصامت لا تعوّض اختبار سماعات سامسونج أو معدل إطاراته أو بطاريته فعليًا.
