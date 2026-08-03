cmake_minimum_required(VERSION 3.18)
project(gossamer_surface)

add_library(gossamer_surface SHARED gossamer_surface.c)
find_library(android-lib android)
target_link_libraries(gossamer_surface ${android-lib})
