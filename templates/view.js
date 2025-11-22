var data = {{.Aliases}};
var changes = {};
var grid;
var domain = "{{.Domain}}";
var tableData = [];
var nextRowId = 0;

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
    for (var rowId in changes) {
        if (changes[rowId]["was"] != null && changes[rowId]["was"] != changes[rowId]["is"]) {
            add_change(cl, "removed " + changes[rowId]["was"]);
            l.push({"op": "remove",
                    "alias": changes[rowId]["was"]});
        }
        if (changes[rowId]["is"] != null) {
            add_change(cl, changes[rowId]["is"] + " \u2192 " + changes[rowId]["target"]);
            l.push({"op": "add",
                    "alias": changes[rowId]["is"],
                    "target": changes[rowId]["target"]});
        }
    }
    document.forms[1].changes.value = JSON.stringify(l);

    // Enable/disable submit button based on whether there are changes
    updateSubmitButton(l.length > 0);
}

function updateSubmitButton(hasChanges) {
    var submitBtn = document.getElementById("submit");
    if (!hasChanges) {
        submitBtn.disabled = true;
        return;
    }

    // Check if all emails are valid
    var allValid = true;
    for (var i = 0; i < tableData.length; i++) {
        var emailValue = tableData[i][0];
        if (emailValue && !validateEmail(emailValue)) {
            allValid = false;
            break;
        }
    }
    submitBtn.disabled = !allValid;
}

function validateEmail(email) {
    // Check if email ends with @domain
    return email.indexOf("@" + domain) !== -1;
}

function validateAllEmails() {
    // Check if there are any changes first
    var hasChanges = Object.keys(changes).length > 0;
    updateSubmitButton(hasChanges);
}

function findRowIndexById(rowId) {
    for (var i = 0; i < tableData.length; i++) {
        if (tableData[i][2] === rowId) {
            return i;
        }
    }
    return -1;
}

function makeEditable(cell, rowId, colIndex) {
    var currentValue = cell.textContent;
    var input = document.createElement('input');
    input.type = 'text';
    input.value = currentValue;
    input.className = 'gridjs-editable-input';
    input.style.width = '100%';
    input.style.boxSizing = 'border-box';
    input.style.padding = '8px';
    input.style.border = '2px solid #007bff';
    input.style.borderRadius = '4px';

    cell.innerHTML = '';
    cell.appendChild(input);
    input.focus();
    input.select();

    function saveEdit() {
        var newValue = input.value.trim();
        var rowIndex = findRowIndexById(rowId);

        if (rowIndex === -1) return;

        var oldValue = tableData[rowIndex][colIndex];
        tableData[rowIndex][colIndex] = newValue;

        // Track changes
        if (colIndex === 0) {
            // Email alias column
            if (changes[rowId] == null) {
                if (oldValue == null || oldValue === "") {
                    changes[rowId] = {"was": newValue};
                } else {
                    changes[rowId] = {"was": oldValue};
                }
                changes[rowId]["is"] = newValue;
                changes[rowId]["target"] = tableData[rowIndex][1] || "";
            } else {
                changes[rowId]["is"] = newValue;
            }
        } else if (colIndex === 1) {
            // Destination column
            if (changes[rowId] == null) {
                var emailValue = tableData[rowIndex][0];
                changes[rowId] = {"was": emailValue, "is": emailValue};
            }
            changes[rowId]["target"] = newValue;
        }

        show_changes();
        validateAllEmails();
        updateGrid();
    }

    input.addEventListener('blur', saveEdit);
    input.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
            e.preventDefault();
            input.blur();
        } else if (e.key === 'Escape') {
            input.value = currentValue;
            input.blur();
        }
    });
}

function removeRow(rowId) {
    var rowIndex = findRowIndexById(rowId);
    if (rowIndex === -1) return;

    var emailValue = tableData[rowIndex][0];

    // Track removal
    if (changes[rowId]) {
        if (changes[rowId]["was"]) {
            changes[rowId] = {
                "was": changes[rowId]["was"],
                "is": null,
                "target": null
            };
        } else {
            delete changes[rowId];
        }
    } else {
        // Existing row being removed
        changes[rowId] = {
            "was": emailValue,
            "is": null,
            "target": null
        };
    }

    tableData.splice(rowIndex, 1);
    show_changes();
    updateGrid();
}

function updateGrid() {
    grid.updateConfig({
        data: tableData.map(function(row) {
            return [row[0], row[1], row[2]];
        })
    }).forceRender();
}

function onload_handler() {
    var container = document.getElementById('spreadsheet');

    // Convert initial data to internal format with row IDs
    tableData = data.map(function(row) {
        var rowId = nextRowId++;
        return [row[0], row[1], rowId];
    });

    // Initialize Grid.js
    grid = new gridjs.Grid({
        columns: [
            {
                name: 'Email alias',
                formatter: function(cell, row) {
                    var rowId = row.cells[2].data;
                    var escaped = (cell || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
                    return gridjs.html('<span class="editable-cell" data-row-id="' + rowId + '" data-col="0">' + escaped + '</span>');
                }
            },
            {
                name: 'Destination(s)',
                formatter: function(cell, row) {
                    var rowId = row.cells[2].data;
                    var escaped = (cell || '').replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
                    return gridjs.html('<span class="editable-cell" data-row-id="' + rowId + '" data-col="1">' + escaped + '</span>');
                }
            },
            {
                name: 'Actions',
                width: '100px',
                formatter: function(cell, row) {
                    var rowId = row.cells[2].data;
                    return gridjs.html('<button class="delete-btn" data-row-id="' + rowId + '">Delete</button>');
                },
                sort: false
            }
        ],
        data: tableData.map(function(row) {
            return [row[0], row[1], row[2]];
        }),
        search: true,
        pagination: {
            limit: 15
        },
        sort: true,
        style: {
            table: {
                'white-space': 'nowrap'
            }
        }
    }).render(container);

    // Handle click events for editing and deletion
    container.addEventListener('click', function(e) {
        var target = e.target;

        // Handle delete button
        if (target.classList.contains('delete-btn')) {
            var rowId = parseInt(target.getAttribute('data-row-id'));
            removeRow(rowId);
            return;
        }

        // Handle editable cell click
        var editableCell = target.closest('.editable-cell');
        if (editableCell) {
            var rowId = parseInt(editableCell.getAttribute('data-row-id'));
            var colIndex = parseInt(editableCell.getAttribute('data-col'));
            var td = editableCell.closest('td');
            if (td) {
                makeEditable(td, rowId, colIndex);
            }
        }
    });

    // Handle add row button
    document.getElementById('add-row-btn').addEventListener('click', function() {
        var rowId = nextRowId++;
        var newRow = ["@" + domain, "", rowId];
        tableData.unshift(newRow);

        // Track the new row
        changes[rowId] = {
            "was": null,
            "is": "@" + domain,
            "target": ""
        };

        show_changes();
        updateGrid();
    });
}

if ("complete" == document.readyState) {
    onload_handler();
} else {
    window.addEventListener("load", onload_handler, false);
}
