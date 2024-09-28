#pragma once

#include <jni.h>

#ifndef _Included_com_dheerajshyam_revcs_Staging
#define _Included_com_dheerajshyam_revcs_Staging

#ifdef __cplusplus

#include <iostream>
#include "nconfig"

using namespace std;

void open_editor(string const& filename);

extern "C" {
#endif // __cplusplus
  
JNIEXPORT void JNICALL Java_com_dheerajshyam_revcs_Staging_open_1editor
(JNIEnv *, jobject, jstring);

#ifdef __cplusplus
}
#endif // __cplusplus

#endif //  _Included_com_dheerajshyam_revcs_Staging
