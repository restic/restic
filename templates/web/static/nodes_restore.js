/*jshint esversion: 6 */
function filesToRestore() {
  const files = [];
  const tree = $("#treeview").fancytree("getTree");
  tree.visit(node => {
    if (node.selected) {
      const path = $(node.tr).data("path");
      files.push(path);
      // if the node is a dir, we don't need to enable childs
      if (node.type === "dir") {
        return "skip";
      }
    }
  });
  return files;
}

$(document).ready(() => {
  $("#restore-form").submit(function(ev) {
    // Stop the browser from submitting the form.
    ev.preventDefault();
    // Api url and values to send
    const restoreApi = $(this).attr("action");
    const restoreData = JSON.stringify({
      target: $("input[name=restore_target]").val(),
      files: filesToRestore()
    });
    const submitButton = $("input[name=submit]");
    submitButton.attr("disabled", true);
    submitButton.val("In Progress...");
    // Submit the form using AJAX.
    $.ajax({
      type: "POST",
      url: restoreApi,
      data: restoreData
    })
      .done(response => {
        const msg = $("#form-messages");
        $(msg).removeClass("alert-danger");
        $(msg).addClass("alert alert-success");
        $(msg).text(response);
        submitButton.attr("disabled", false);
        submitButton.val("Restore");
      })
      .fail(data => {
        const msg = $("#form-messages");
        $(msg).removeClass("alert-success");
        $(msg).addClass("alert alert-danger");
        $(msg).text(data.responseText);
        submitButton.attr("disabled", false);
        submitButton.val("Restore");
      });
  });
});
