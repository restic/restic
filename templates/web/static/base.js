function human_filesize(bytes) {
  var i = -1;
  var byteUnits = [" kB", " MB", " GB", " TB", " PB", " EB", " ZB", " YB"];
  do {
    bytes = bytes / 1024;
    i++;
  } while (bytes > 1024);

  return Math.max(bytes, 0.1).toFixed(1) + byteUnits[i];
}

function human_date(datetime) {
  return moment(datetime).format("YYYY/MM/DD HH:MM:SS");
}

function array_to_div(arr) {
  var r = "";
  $.each(arr, function(i, a) {
    r += `<div>${a}</div>`;
  });
  return r;
}
