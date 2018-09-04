/*jshint esversion: 6 */
function humanFileSize(bytes) {
  let i = -1;
  const byteUnits = [" kB", " MB", " GB", " TB", " PB", " EB", " ZB", " YB"];
  do {
    bytes = bytes / 1024;
    i++;
  } while (bytes > 1024);

  return Math.max(bytes, 0.1).toFixed(1) + byteUnits[i];
}

function humanDate(datetime) {
  return moment(datetime).format("YYYY/MM/DD HH:MM:SS");
}

function arrayToDiv(arr) {
  let r = "";
  $.each(arr, (i, a) => {
    r += `<div>${a}</div>`;
  });
  return r;
}
