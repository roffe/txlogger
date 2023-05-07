var socket = io("ws://localhost:8080", {
    transports: ['websocket'],
});
var graphs = {}, redrawInterval, symbolAssignments = {};

function redraw() {
    $.each(graphs, (id, graph) => {
        graph.redraw();
    });
}

function addSeriesPoint(timestamp, id, value) {
    id = id.toString();
    if (symbolAssignments[id]) {
        symbolAssignments[id].series.addPoint([timestamp, 1 * value], false, false, false);
    }
}

function getGraphId(symbolData) {
    if (symbolData.Group) {
        return symbolData.Group;
    } else {
        return 'NoGroup-' + symbolData.ID.toString();
    }
}

socket.on("metrics", data => {
    //console.log('metrics', JSON.stringify(data));
    if (typeof (data) === 'string') {
        const split = data.split('|');
        const timestamp = Date.parse(split[0])
        $.each(split[1].split(','), (key, val) => {
            const value = val = val.split(':');
            addSeriesPoint(timestamp, value[0], value[1]);
        });
    }
});

socket.on("symbol_list", data => {
    console.log('Symbols', data);
    if (data !== null) {
        $.each(graphs, graph => {
            if (typeof graph.destroy === 'function') {
                graph.destroy();
            }
        });
        $('#container').empty();
        graphs = {};
        symbolAssignments = {};

        $.each(data, (key, val) => {
            const graphId = getGraphId(val);
            if (!graphs[graphId]) {
                if (val.Group) {
                    graphs[graphId] = createGraph(val.Group);
                } else {
                    graphs[graphId] = createGraph(val.Name);
                }
            }
            symbolAssignments[val.ID.toString()] = {
                unit: val.Unit,
                type: val.Type,
                name: val.Name,
                group: val.Group,
                graph: graphs[graphId],
                series: createNewSeries(graphs[graphId], val.Type, val.Unit, val.Name),
            };
        });

        socket.emit('start_session');
        redrawInterval = setInterval(redraw, 1000);
    } else {
        console.log('No symbols!');
    }
});

socket.on('connect', () => {
    console.log('Socket connected!');
    $('#loading-spinner').remove();
    clearInterval(redrawInterval);

    socket.emit('request_symbols');
});