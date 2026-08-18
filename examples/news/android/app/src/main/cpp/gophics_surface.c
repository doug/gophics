// NDK helper: hand the Go/GPU side the ANativeWindow* behind a Java Surface,
// which Java can't produce on its own. Built via CMakeLists.txt in this dir.
#include <jni.h>
#include <stdint.h>
#include <android/native_window_jni.h>

JNIEXPORT jlong JNICALL
Java_dev_gophics_news_NativeSurface_acquire(JNIEnv *env, jobject thiz, jobject surface) {
    (void) thiz;
    if (surface == NULL) return 0;
    // ANativeWindow_fromSurface acquires a reference; released in release().
    return (jlong) (uintptr_t) ANativeWindow_fromSurface(env, surface);
}

JNIEXPORT void JNICALL
Java_dev_gophics_news_NativeSurface_release(JNIEnv *env, jobject thiz, jlong ptr) {
    (void) env;
    (void) thiz;
    if (ptr != 0) ANativeWindow_release((ANativeWindow *) (uintptr_t) ptr);
}
