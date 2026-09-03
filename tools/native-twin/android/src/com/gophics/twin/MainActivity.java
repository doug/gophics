package com.gophics.twin;

// The Android twin: a ScrollView — which flings through OverScroller — over the
// same scene tools/uitrace replays through gophics, recording one flick into
// the trace contract. Java, not Kotlin, because javac ships with the JDK and
// the build is four SDK tools with no Gradle in between.
//
// The finger phase is every MOVE event's delta in density-independent pixels,
// which is what gophics's logical pixels are on Android; the offset is
// getScrollY() sampled once per Choreographer frame. When the view has been
// still for a few frames after the finger lifted, the trace is written to the
// app's external files dir (adb pull reaches it without root) and the process
// exits. One run, one gesture, one file.

import android.app.Activity;
import android.graphics.Color;
import android.os.Bundle;
import android.os.SystemClock;
import android.util.Log;
import android.view.Choreographer;
import android.view.MotionEvent;
import android.view.ViewGroup;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;
import java.io.File;
import java.io.FileWriter;
import java.util.ArrayList;
import java.util.List;

public class MainActivity extends Activity {
    static final String TAG = "twin";
    ScrollView sv;
    float density;
    double t0 = -1, releaseT = 0, lastTouchY = 0, lastY = Double.NaN;
    boolean released = false, finished = false;
    int quiet = 0;
    final List<double[]> input = new ArrayList<>(), offset = new ArrayList<>();

    @Override protected void onCreate(Bundle b) {
        super.onCreate(b);
        density = getResources().getDisplayMetrics().density;
        sv = new ScrollView(this);
        sv.setVerticalScrollBarEnabled(true);
        LinearLayout col = new LinearLayout(this);
        col.setOrientation(LinearLayout.VERTICAL);
        int rowH = Math.round(44 * density), padX = Math.round(16 * density), padY = Math.round(12 * density);
        for (int i = 0; i < 300; i++) {
            TextView tv = new TextView(this);
            tv.setText("Row " + i);
            tv.setTextSize(16);
            tv.setTextColor(Color.rgb(26, 26, 31));
            tv.setPadding(padX, padY, padX, 0);
            tv.setBackgroundColor(i % 2 == 1 ? Color.rgb(230, 232, 240) : Color.rgb(245, 245, 247));
            col.addView(tv, new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, rowH));
        }
        sv.addView(col);
        setContentView(sv);
        Log.i(TAG, "twin ready — flick upward once (adb shell input swipe works)");
    }

    @Override public boolean dispatchTouchEvent(MotionEvent e) {
        if (!released) {
            double t = e.getEventTime() / 1000.0;
            switch (e.getActionMasked()) {
                case MotionEvent.ACTION_DOWN:
                    t0 = t; lastTouchY = e.getY();
                    startFrames();
                    break;
                case MotionEvent.ACTION_MOVE:
                    if (t0 >= 0) {
                        input.add(new double[]{t - t0, (e.getY() - lastTouchY) / density});
                        lastTouchY = e.getY();
                    }
                    break;
                case MotionEvent.ACTION_UP:
                case MotionEvent.ACTION_CANCEL:
                    if (t0 >= 0) { released = true; releaseT = t - t0; }
                    break;
            }
        }
        return super.dispatchTouchEvent(e);
    }

    void startFrames() {
        sample();
        Choreographer.getInstance().postFrameCallback(new Choreographer.FrameCallback() {
            @Override public void doFrame(long nanos) {
                sample();
                if (!finished) Choreographer.getInstance().postFrameCallback(this);
            }
        });
    }

    void sample() {
        if (t0 < 0 || finished) return;
        double t = SystemClock.uptimeMillis() / 1000.0 - t0; // same clock as event times
        double y = sv.getScrollY() / density;
        offset.add(new double[]{t, y});
        if (released) {
            quiet = (y == lastY) ? quiet + 1 : 0;
            if ((quiet >= 3 && offset.size() > 6) || quiet >= 60) finishTrace();
        }
        lastY = y;
    }

    void finishTrace() {
        finished = true;
        double hz = offset.size() > 2
            ? Math.round((offset.size() - 1) / (offset.get(offset.size() - 1)[0] - offset.get(0)[0])) : 0;
        // Contract: finger up is negative input and offset rises; normalize.
        double inSum = 0; for (double[] s : input) inSum += s[1];
        double travel = offset.get(offset.size() - 1)[1] - offset.get(0)[1];
        String note = "Android " + android.os.Build.VERSION.RELEASE + " emulator (" + android.os.Build.MODEL
            + "), ScrollView/OverScroller, density " + density + "; offset per Choreographer frame";
        boolean flip = inSum != 0 && travel != 0 && (inSum > 0) == (travel > 0);
        if (flip) note += "; input sign flipped to the contract's convention";
        StringBuilder j = new StringBuilder();
        j.append("{\"source\":\"android-overscroller\",\"hz\":").append((int) hz)
         .append(",\"notes\":\"").append(note).append("\",\"release_t\":").append(releaseT).append(",\"input\":[");
        for (int i = 0; i < input.size(); i++) {
            double[] s = input.get(i);
            j.append(i > 0 ? "," : "").append("{\"t\":").append(s[0]).append(",\"v\":").append(flip ? -s[1] : s[1]).append("}");
        }
        j.append("],\"offset\":[");
        for (int i = 0; i < offset.size(); i++) {
            double[] s = offset.get(i);
            j.append(i > 0 ? "," : "").append("{\"t\":").append(s[0]).append(",\"v\":").append(s[1]).append("}");
        }
        j.append("]}");
        try {
            File f = new File(getExternalFilesDir(null), "trace.json");
            FileWriter w = new FileWriter(f); w.write(j.toString()); w.close();
            Log.i(TAG, "wrote " + f + ": " + input.size() + " input events, " + offset.size() + " frames at "
                + (int) hz + "Hz, release at " + releaseT + "s, travel " + (int) travel + "dp");
        } catch (Exception ex) { Log.e(TAG, "write failed", ex); }
        Log.i(TAG, "TRACE-DONE");
        finishAffinity();
        new android.os.Handler().postDelayed(() -> System.exit(0), 300);
    }
}
