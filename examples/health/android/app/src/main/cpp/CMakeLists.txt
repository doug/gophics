cmake_minimum_required(VERSION 3.18)
project(gophics_surface)

add_library(gophics_surface SHARED gophics_surface.c)
find_library(android-lib android)
target_link_libraries(gophics_surface ${android-lib})

# Align LOAD segments to 16 KB. Devices from the Pixel 10 on can boot with 16 KB
# pages (ro.product.cpu.pagesize.max=16384); the NDK's 4 KB default makes Android
# show a "not compatible" PageSizeMismatch dialog at launch on those. Note this
# is ELF segment alignment, not APK zip alignment — zipalign -P 16 passes either
# way, so the dialog is the only symptom.
target_link_options(gophics_surface PRIVATE "-Wl,-z,max-page-size=16384")
