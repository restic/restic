/*jshint esversion: 6 */
function convertData(data) {
  // work with the childs "nodes"
  nodes = data.nodes;
  // adjust field and fix struct
  nodes = $.map(nodes, c => {
    // delete unused attributes
    delete c.atime;
    delete c.ctime;
    delete c.device_id;
    delete c.inode;
    delete c.mode;
    // rename name to title
    c.title = c.name;
    // delete c.name;
    // should we check if type == dir instead ?
    if (c.hasOwnProperty("subtree")) {
      c.folder = true;
      c.lazy = true;
    }
    return c;
  });
  return nodes;
}

function pathFromRoot(node) {
  if (node.parent === null) {
    return "";
  }
  return `${pathFromRoot(node.parent)}/${node.data.name}`;
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
      url: `/api/snapshots/${$("#treeview").data("snapshot-id")}/nodes/`
    },
    lazyLoad(event, data) {
      const node = data.node;
      const snapshotID = node.tree.data["snapshot-id"];
      const nodeKey = node.data.subtree;
      data.result = {
        url: `/api/snapshots/${snapshotID}/nodes/${nodeKey}`
      };
    },
    // apply parent's state to new child nodes
    loadChildren(event, data) {
      data.node.fixSelection3AfterClick();
    },
    // This event is part of the table extension:
    renderColumns(event, data) {
      const n = data.node.data;
      const path = pathFromRoot(data.node);
      const td = $(data.node.tr).find(">td");

      $(data.node.tr)[0].setAttribute("data-path", path);

      td.eq(1).attr("title", path);
      td.eq(2).text(n.user);
      td.eq(3).text(n.group);
      // mtime
      td.eq(4).text(humanDate(n.mtime));
      // size
      if (n.hasOwnProperty("size")) {
        td.eq(5).text(humanFileSize(n.size));
      }
      // actions
      if (data.node.type !== "dir") {
        const snapshotID = data.tree.data["snapshotId"];
        const link = document.createElement("a");
        link.text = "Download";
        link.href = `/web/snapshots/${snapshotID}/download?path=${path}`;
        td.eq(6).append(link);
      }
    },
    postProcess(event, data) {
      data.result = convertData(data.response);
    }
  });
}

$(document).ready(readyFn);
