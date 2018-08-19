function readyFn(jQuery) {
  // initialize the treeview with fancytree
  $("#treeview").fancytree({
    extensions: ["table"],
    table: {
      indentation: 5,
      nodeColumnIdx: 1,
      checkboxColumnIdx: 0
    },
    checkbox: true,
    source: {
      url: "/api/snapshots/" + $("#treeview").data("snapshot-id") + "/nodes/"
    },
    lazyLoad: function(event, data) {
      var node = data.node;
      var snapshot_id = node.tree.data["snapshot-id"];
      var node_key = node.key;
      data.result = {
        url: "/api/snapshots/" + snapshot_id + "/nodes/" + node_key
      };
    },
    // This event is part of the table extension:
    renderColumns: function(event, data) {
      var node = data.node;

      $tdList = $(node.tr).find(">td");

      $tdList.eq(2).text(node.data.user);
      $tdList.eq(3).text(node.data.group);
      // mtime
      $tdList.eq(4).text(human_date(node.data.mtime));
      // size
      if (node.data.hasOwnProperty("size")) {
        $tdList.eq(5).text(human_filesize(node.data.size));
      }
      // actions
      // if (node.data.hasOwnProperty("size")) {
      //   var link = document.createElement("a");
      //   link.href = "/snapshots/";
      //   link.text = "D";
      //   $tdList.eq(6).append(link);
      // }
    },
    postProcess: function(event, data) {
      data.result = convertData(data.response);
    }
  });

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
