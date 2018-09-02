function readyFn(jQuery) {
  $.getJSON("/api/snapshots/", function(data) {
    data = data.sort(function(a, b) {
      var x = a.time;
      var y = b.time;
      return x < y ? -1 : x > y ? 1 : 0;
    });
    // console.log(data);
    $.each(data, function(k, v) {
      $("#table-snapshots tbody").append(
        `<tr>
          <td>
            <a href="/web/snapshots/${v.short_id}/nodes/">${v.short_id}<a>
          </td>
          <td>${human_date(v.time)}</td>
          <td>${v.username}</td>
          <td>${v.hostname}</td>
          <td>${array_to_div(v.paths)}</td>
          <td>${array_to_div(v.tags)}</td>
        </tr>`
      );
    });
  });
}

$(document).ready(readyFn);
