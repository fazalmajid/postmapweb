var data = {{.Aliases}};
var changes = {};
function add_change(cl, s) {
    var n = document.createElement("li");
    var n2 = document.createTextNode(s);
    n.appendChild(n2);
    cl.appendChild(n);
}
function show_changes() {
    var cl = document.getElementById("changelog");
    var l = [];
    cl.innerHTML = "";
    for (var cell in changes) {
        if (changes[cell]["was"] != null && changes[cell]["was"] != changes[cell]["is"]) {
            add_change(cl, "removed " + changes[cell]["was"]);
            l.push({"op": "remove",
                    "alias": changes[cell]["was"]});
        }
        add_change(cl, changes[cell]["is"] + " \u2192 " + changes[cell]["target"]);
        l.push({"op": "add",
                "alias": changes[cell]["is"],
                "target": changes[cell]["target"]});
    }
    document.forms[0].changes.value = JSON.stringify(l);
}
function change_handler(change, source) {
    if (source === 'loadData') {
        return; //don't save this change
    }
    var i;
    for(i=0; i<change.length; i++) {
        var cell = change[i][0];
        var prop = change[i][1];
        var oldVal = change[i][2];
        var newVal = change[i][3];
        if (prop == 0) {
            if (changes[cell] == null) {
                if (oldVal == null) {
                    changes[cell] = {"was": newVal};
                } else {
                    changes[cell] = {"was": oldVal};
                }
                changes[cell]["is"] = newVal;
                changes[cell]["target"] = data[cell][1];
            }
            if (newVal.indexOf("@{{.Domain}}") == -1) {
                hot.getCell(cell, 0).style.backgroundColor = "#fc9";
                document.getElementById("submit").disabled = true;
                errored[cell] = true;
            } else {
                hot.getCell(cell, 0).style.backgroundColor = "#fff";
                if (errored[cell]) {
                    delete errored[cell];
                    hot.getCell(cell, 0).style.backgroundColor = "#fff";
                    if (Object.keys(errored).length == 0) {
                        document.getElementById("submit").disabled = false;
                    }
                }
            }
        } else {
            if (changes[cell] == null) {
                changes[cell] = {"was": data[cell][0], "is": data[cell][0]};
            }
            changes[cell]["target"] = newVal;
        }
    }
    show_changes();
}
function check_pending() {
    var e = hot.getActiveEditor();
    if (e.state == "STATE_EDITING" && e.TEXTAREA.value != e.originalValue) {
        change_handler([[e.row, e.col, e.originalValue, e.TEXTAREA.value]],
                       "pending");
    }
    return true;
}

//document.getElementById("submit").onsubmit = check_pending;
var errored = {};
var hot;
function onload_handler() {
    var container = document.getElementById('spreadsheet');
    hot = new Handsontable(container, {
        data: data,
        minSpareRows: 1,
        rowHeaders: true,
        colHeaders: ["Email alias", "Destination(s)"],
        contextMenu: false,
        afterChange: change_handler,
    });
}
if ( "complete" == document.readyState ) {
    onload_handler();
} else {
    window.addEventListener("load", onload_handler, false);
}
