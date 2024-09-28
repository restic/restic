#include "main.h"

void open_editor(string const& filename) {
  string editor = "vi";

  int pid = fork();

  if(pid == -1)
    goto returnErr;
  else if(pid == 0) {
    execlp(editor.c_str(), editor.c_str(), filename.c_str(), NULL);
    goto returnErr;
  } else wait(nullptr);

  return;
  
  returnErr: {
    cerr << "Unable to call editor right now, exiting with status -1." << endl;
    flush(cerr);
    exit(-1);
  }
}

JNIEXPORT void JNICALL Java_com_dheerajshyam_revcs_Staging_open_1editor
    (JNIEnv *env, jobject thisObj, jstring file_name) {

  jboolean isCopy = true;
  auto c_file_name = env->GetStringUTFChars(file_name, &isCopy);

  open_editor(string(c_file_name));
}
