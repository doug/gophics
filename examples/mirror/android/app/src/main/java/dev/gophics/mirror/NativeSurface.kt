package dev.gophics.mirror

import android.view.Surface

/**
 * NativeSurface bridges a Java [Surface] to its native ANativeWindow pointer,
 * which the Go/GPU side needs to create a Vulkan surface. Backed by a tiny NDK
 * helper (app/src/main/cpp/gophics_surface.c); wire it up in build.gradle:
 *
 *   android { externalNativeBuild { cmake { path = file("src/main/cpp/CMakeLists.txt") } } }
 */
object NativeSurface {
    init { System.loadLibrary("gophics_surface") }

    /** Returns the ANativeWindow* (as a long) for surface; call release when done. */
    external fun acquire(surface: Surface): Long

    /** Releases an ANativeWindow* previously returned by acquire. */
    external fun release(ptr: Long)
}
