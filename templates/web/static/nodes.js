function convertData(data) {
  // work with the childs "nodes"
  data = data.nodes;
  // adjust field and fix struct
  data = $.map(data, function(c) {
    // delete unused attributes
    delete c.atime;
    delete c.ctime;
    delete c.device_id;
    delete c.inode;
    delete c.mode;
    // rename name to title
    c.title = c.name;
    delete c.name;
    // should we check if type == dir instead ?
    if (c.hasOwnProperty("subtree")) {
      c.key = c.subtree;
      c.folder = true;
      c.lazy = true;
    }
    return c;
  });
  return data;
}

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
    selectMode: 3,
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
      var n = data.node.data;
      $tdList = $(data.node.tr).find(">td");

      $tdList.eq(2).text(n.user);
      $tdList.eq(3).text(n.group);
      // mtime
      $tdList.eq(4).text(human_date(n.mtime));
      // size
      if (n.hasOwnProperty("size")) {
        $tdList.eq(5).text(human_filesize(n.size));
      }
      // actions
      // if (n.hasOwnProperty("size")) {
      //   var snapshot_id = node.tree.data["snapshotId"];
      //   var link = document.createElement("a");
      //   link.text = "Download";
      //   link.href = "";
      //   link.dataset.content = n.content;
      //   $tdList.eq(6).append(link);
      // }
    },
    postProcess: function(event, data) {
      data.result = convertData(data.response);
    }
  });
}

$(document).ready(readyFn);
