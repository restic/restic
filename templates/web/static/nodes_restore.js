function files_to_restore() {
  var files = [];
  var tree = $("#treeview").fancytree("getTree");
  tree.visit(function(node) {
    if (node.selected) {
      var path = $(node.tr).data("path");
      files.push(path);
      // if the node is a dir, we don't need to enable childs
      if (node.type === "dir") {
        return "skip";
      }
    }
  });
  return files;
}

$(document).ready(function() {
  $("#restore-form").submit(function(ev) {
    // Stop the browser from submitting the form.
    ev.preventDefault();
    // Api url and values to send
    var restore_api = $(this).attr("action");
    var restore_data = JSON.stringify({
      target: $("input[name=restore_target]").val(),
      files: files_to_restore()
    });
    var submit_button = $("input[name=submit]");
    submit_button.attr("disabled", true);
    submit_button.val("In Progress...");
    // Submit the form using AJAX.
    $.ajax({
      type: "POST",
      url: restore_api,
      data: restore_data
    })
      .done(function(response) {
        var msg = $("#form-messages");
        $(msg).removeClass("alert-danger");
        $(msg).addClass("alert alert-success");
        $(msg).text(response);
        submit_button.attr("disabled", false);
        submit_button.val("Restore");
      })
      .fail(function(data) {
        var msg = $("#form-messages");
        $(msg).removeClass("alert-success");
        $(msg).addClass("alert alert-danger");
        $(msg).text(data.responseText);
        submit_button.attr("disabled", false);
        submit_button.val("Restore");
      });
  });
});
