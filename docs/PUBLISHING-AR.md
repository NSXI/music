# بناء ونشر نَوى

`desktop/` يحتوي مصدر ويندوز. يستخدم `android/` الواجهة والمنطق المشترك مع مشغّل أندرويد الأصلي. ينقل سكربت المزامنة التغييرات المشتركة ويحافظ على تغييرات أندرويد المحلية ويبلغك بالتعارضات.

## بناء نسخ الاختبار

تستخدم خطوات المستودع Go 1.26 وPython 3.11 فأحدث وJDK 17 وAndroid SDK 35. يحدد [مسار GitHub Actions](../.github/workflows/verify.yml) إصدارات NDK وCMake وأدوات البناء، وتحتاج اختبارات الصوت إلى FFmpeg في `PATH`.

```powershell
python android/scripts/sync_desktop.py --apply
python android/scripts/test_shared_sync.py
go test -C desktop ./...
go build -C desktop -tags 'desktop,production' -ldflags '-s -w -H windowsgui' -o ../dist/Nawa.exe .
./android/build-core.ps1 -Abi arm64-v8a
./android/gradlew.bat -p android -PnawaAbis=arm64-v8a :app:assembleDebug :app:lintDebug
```

تعمل نسخة Android Debug تحت اسم ينتهي بـ`.debug`. لاختبار ويندوز، شغّل الحزمة مع أدواتها في مجلد معزول. ينشئ GitHub Actions حزم اختبار من المصدر.

## تجهيز إصدار عام

1. ارفع رقم الإصدار و`versionCode` في `desktop/release.json` و`desktop/release_identity.go`. زامن المصدر وابنِ واختبر المنصتين.
2. وقّع APK بمفتاح الإصدار نفسه للمحافظة على إمكانية الترقية. يقرأ بناء Release مسار المخزن `NAWA_SIGNING_STORE` وكلمة مروره `NAWA_SIGNING_PASSWORD` من بيئة العملية. احتفظ بالمفتاح ونسخته الاحتياطية في مكان خاص.
3. تحقّق من توقيع APK ثم سجّل بصمة البناء باستخدام `python android/scripts/build_identity.py --record-release <apk>`. جهّز ZIP ويندوز وفيه `Nawa.exe` وFFmpeg وFFprobe وyt-dlp وNode وتراخيصها. تضم الحزم ملفات التوزيع فقط.
4. أنشئ مسودة إصدار تستهدف مراجعة المصدر المطابقة. ارفع ZIP وAPK و`nawa-update.json` و`NAWA-SHA256SUMS.txt`. راجع الحجم وSHA-256 لكل أصل مرفوع. يحصل المستخدم على مصدر الإصدار من أرشيف الوسم الذي يولّده GitHub.
5. يحتوي ملف التحديث على `schema: 1` و`appId: dev.nawa.music` و`versionCode` و`sharedRevision`، وروابط HTTPS لحزمتَي `windows-x64` و`android-arm64` مع الحجم والبصمة. أضف رابطًا ثابتًا وبصمة لكل محرك اختياري متوافق. يستخدم 3.0 حزم محركات التوقيت والترجمة التي سبق التحقق منها.
6. بعد النشر تحقّق من أن [ملف التحديث العام](https://github.com/NSXI/music/releases/latest/download/nawa-update.json) يشير إلى الإصدار الصحيح، وأن التنزيلات تفتح من دون حساب. يظهر التحديث للأجهزة التي تحمل رقم إصدار أقدم، ويؤكد المستخدم التثبيت على أندرويد.
