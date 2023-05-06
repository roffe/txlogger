var socket = io("ws://localhost:8080", {
    transports: ['websocket'],
});
var graphs = {}

var refresh = false;
var cnt = 0;

socket.on("metrics", data => {
    //console.log('metrics', JSON.stringify(data));
    if (typeof (data) === 'string') {
        let tups = data = data.split(',');
        if (cnt === 5) {
            refresh = true;
            cnt = 0;
        } else {
            refresh = false;
        }
        $.each(tups, (key, val) => {
            const value = val.split(':');
            addSeriesPoint(value[0], value[1], refresh);
        });
        cnt++;
    } else if (data !== null && typeof (data) === 'object') {
        $.each(data, (key, val) => {
            val = val.split(':');
            addSeriesPoint(val[0], val[1], true);
        });
    }
});


function addSeriesPoint(id, value, refresh) {
    id = id.toString();
    if (graphs[id]) {
        var x = (new Date()).getTime(); // current time (TODO?)
        //debugger;
        graphs[id].series[0].addPoint([x, 1 * value], refresh, false, true)
    }
}

socket.on("symbol_list", data => {
    console.log('Symbols', data);
    if (data !== null) {
        $.each(graphs, graph => {
            graph.destroy();
        });
        $('#container>.chart').remove();
        graphs = {};

        $.each(data, (key, val) => {
            console.log(val);
            graphs[val.ID.toString()] = createGraph(val);
        });

        socket.emit('start_session');
    } else {
        console.log('No symbols!');
    }
});

socket.on('connect', () => {
    console.log('Socket connected!');
    $('#loading-spinner').remove();

    socket.emit('request_symbols');
});