// Reference iOS MonitorHost for gophics's shell/mobile live-capture bridge.
//
// Wire once at startup:  bridge.setMonitorHost(GophicsMonitor(bridge: bridge))
//
// This is the streaming counterpart to GophicsMedia: MediaHost records one clip
// and hands it back when it stops, while MonitorHost keeps an open stream the Go
// side analyses live. A tuner or a singing app needs the latter — the audio has
// to be current, not eventual.

import AVFoundation
import Mobile
import Foundation

final class GophicsMonitor: NSObject {
    private let bridge: MobileBridge
    private let engine = AVAudioEngine()
    private var active = false
    private var reqID = 0

    init(bridge: MobileBridge) {
        self.bridge = bridge
    }

    private func ui(_ f: @escaping () -> Void) { DispatchQueue.main.async(execute: f) }
}

extension GophicsMonitor: MobileMonitorHostProtocol {

    // MARK: Permission

    func authorizeMic(_ reqID: Int) {
        AVAudioSession.sharedInstance().requestRecordPermission { granted in
            self.ui { self.bridge.deliverPermission(reqID, granted: granted) }
        }
    }

    // MARK: Monitoring

    func startMonitoring(_ reqID: Int) {
        if active { stopMonitoring(reqID) }
        self.reqID = reqID

        let session = AVAudioSession.sharedInstance()
        do {
            // .measurement is the point of this whole configuration: it turns
            // off the input processing — AGC, noise suppression, the voice EQ —
            // that iOS otherwise applies. That processing is tuned to make
            // speech intelligible and it distorts the harmonic structure pitch
            // detection reads, so a tuner using the default mode reads a voice
            // it has already altered.
            try session.setCategory(.playAndRecord, mode: .measurement,
                                    options: [.defaultToSpeaker, .allowBluetooth])
            try session.setActive(true)
        } catch {
            ui { self.bridge.failMonitoring(reqID, msg: "audio session: \(error.localizedDescription)") }
            return
        }

        let input = engine.inputNode
        let format = input.inputFormat(forBus: 0)
        guard format.sampleRate > 0, format.channelCount > 0 else {
            ui { self.bridge.failMonitoring(reqID, msg: "no audio input available") }
            return
        }

        input.installTap(onBus: 0, bufferSize: 1024, format: format) { [weak self] buffer, _ in
            guard let self, self.active else { return }
            guard let channels = buffer.floatChannelData else { return }

            let frames = Int(buffer.frameLength)
            if frames == 0 { return }
            let chCount = Int(buffer.format.channelCount)

            // Downmix to mono: the analyser is mono, and taking only channel 0
            // would silently halve the level on a stereo input.
            var mono = [Float](repeating: 0, count: frames)
            for c in 0..<chCount {
                let src = channels[c]
                for i in 0..<frames { mono[i] += src[i] }
            }
            if chCount > 1 {
                let inv = 1 / Float(chCount)
                for i in 0..<frames { mono[i] *= inv }
            }

            // Little-endian float32 bytes: gomobile binds only byte slices, so
            // a [Float] parameter is not expressible across the boundary.
            //
            // Delivered straight from the render thread on purpose. This
            // touches only a mutex-guarded ring buffer on the Go side and runs
            // hundreds of times a second; hopping to the main queue would add
            // latency to the one path that must stay current.
            mono.withUnsafeBufferPointer { p in
                let data = Data(buffer: p)
                self.bridge.deliverMonitorFloat32(self.reqID, data: data)
            }
        }

        engine.prepare()
        do {
            try engine.start()
        } catch {
            input.removeTap(onBus: 0)
            ui { self.bridge.failMonitoring(reqID, msg: "engine: \(error.localizedDescription)") }
            return
        }

        active = true
        let rate = Int(format.sampleRate)
        ui { self.bridge.deliverMonitorReady(reqID, sampleRate: rate) }
    }

    func stopMonitoring(_ reqID: Int) {
        guard active else { return }
        active = false
        engine.inputNode.removeTap(onBus: 0)
        engine.stop()
        // Give the session back so other apps — and this app's own playback —
        // are not left in a record-configured route.
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }
}
