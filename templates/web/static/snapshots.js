/*jshint esversion: 6 */
function readyFn(jQuery) {
  $.getJSON("/api/snapshots/", data => {
    data = data.sort((a, b) => {
      const x = a.time;
      const y = b.time;
      return x < y ? -1 : x > y ? 1 : 0;
    });
    // console.log(data);
    $.each(data, (k, v) => {
      $("#table-snapshots tbody").append(
        `<tr>
          <td>
            <a href="/web/snapshots/${v.short_id}/nodes/">${v.short_id}<a>
          </td>
          <td>${humanDate(v.time)}</td>
          <td>${v.username}</td>
          <td>${v.hostname}</td>
          <td>${arrayToDiv(v.paths)}</td>
          <td>${arrayToDiv(v.tags)}</td>
        </tr>`
      );
    });
  });
}

$(document).ready(readyFn);
