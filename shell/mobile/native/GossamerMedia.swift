// Reference iOS MediaHost for gossamer's shell/mobile media bridge.
// Scaffolding — copy into the host app and verify on device. Type names from
// `gomobile bind` (here `Mobile...`) may differ; adjust to your bind package.
//
// Wire once at startup:  bridge.setMediaHost(GossamerMedia(bridge: bridge))

import UIKit
import AVFoundation

final class GossamerMedia: NSObject {
    private let bridge: MobileBridge
    private weak var presenter: UIViewController?

    // Camera
    private var captureReq = 0
    // Recording
    private let engine = AVAudioEngine()
    private var recReq = 0
    private var recRate = 0
    private var recPCM = Data()
    private let pcmQueue = DispatchQueue(label: "gossamer.pcm")
    private var recStart = Date()
    // Playback
    private var player: AVAudioPlayer?
    private var playReq = 0
    private var posTimer: Timer?

    init(bridge: MobileBridge, presenter: UIViewController?) {
        self.bridge = bridge
        self.presenter = presenter
    }

    private func ui(_ f: @escaping () -> Void) { DispatchQueue.main.async(execute: f) }
}

extension GossamerMedia: MobileMediaHost {
    // MARK: Camera (UIImagePickerController — native camera UI, like web input-capture)

    func authorizeCamera(_ reqID: Int) {
        // Camera via the picker needs only the Info.plist usage string.
        bridge.deliverPermission(reqID, granted: true)
    }

    func capturePhoto(_ reqID: Int, facing: Int) {
        captureReq = reqID
        guard UIImagePickerController.isSourceTypeAvailable(.camera) else {
            bridge.failCapture(reqID, msg: "no camera"); return
        }
        let p = UIImagePickerController()
        p.sourceType = .camera
        p.cameraDevice = (facing == 1) ? .front : .rear
        p.delegate = self
        presenter?.present(p, animated: true)
    }

    // MARK: Audio

    func authorizeMic(_ reqID: Int) {
        AVAudioSession.sharedInstance().requestRecordPermission { ok in
            self.ui { self.bridge.deliverPermission(reqID, granted: ok) }
        }
    }

    func startRecording(_ reqID: Int) {
        AVAudioSession.sharedInstance().requestRecordPermission { ok in
            self.ui {
                guard ok else { self.bridge.failRecording(reqID, msg: "microphone denied"); return }
                do {
                    try AVAudioSession.sharedInstance().setCategory(.record)
                    try AVAudioSession.sharedInstance().setActive(true)
                    let input = self.engine.inputNode
                    let fmt = input.outputFormat(forBus: 0)
                    self.recRate = Int(fmt.sampleRate)
                    self.recPCM = Data(); self.recReq = reqID; self.recStart = Date()
                    input.installTap(onBus: 0, bufferSize: 4096, format: fmt) { buf, _ in
                        guard let ch = buf.floatChannelData?[0] else { return }
                        let n = Int(buf.frameLength)
                        var pcm = Data(capacity: n * 2); var peak: Float = 0
                        for i in 0..<n {
                            var s = ch[i]
                            if s > 1 { s = 1 }; if s < -1 { s = -1 }
                            if abs(s) > peak { peak = abs(s) }
                            var v = Int16(s * 32767).littleEndian
                            withUnsafeBytes(of: &v) { pcm.append(contentsOf: $0) }
                        }
                        self.pcmQueue.sync { self.recPCM.append(pcm) }
                        self.ui { self.bridge.setAudioLevel(reqID, level: peak) }
                    }
                    try self.engine.start()
                    self.bridge.deliverRecorderReady(reqID)
                } catch {
                    self.bridge.failRecording(reqID, msg: "\(error)")
                }
            }
        }
    }

    func stopRecording(_ reqID: Int) {
        engine.inputNode.removeTap(onBus: 0)
        engine.stop()
        let ms = Int(Date().timeIntervalSince(recStart) * 1000)
        let pcm = pcmQueue.sync { self.recPCM }
        bridge.deliverPCM(reqID, pcm: pcm, sampleRate: recRate, durationMs: ms)
    }

    func playClip(_ reqID: Int, wav: Data) {
        do {
            try AVAudioSession.sharedInstance().setCategory(.playback)
            try AVAudioSession.sharedInstance().setActive(true)
            let p = try AVAudioPlayer(data: wav)
            p.delegate = self
            player = p; playReq = reqID
            p.play()
            bridge.deliverPlaybackReady(reqID)
            posTimer?.invalidate()
            posTimer = Timer.scheduledTimer(withTimeInterval: 0.05, repeats: true) { _ in
                if let p = self.player { self.bridge.setPlaybackPosition(reqID, ms: Int(p.currentTime * 1000)) }
            }
        } catch {
            bridge.playbackEnded(reqID)
        }
    }

    func seekPlayback(_ reqID: Int, ms: Int) { player?.currentTime = Double(ms) / 1000 }

    func stopPlayback(_ reqID: Int) {
        player?.stop(); player = nil
        posTimer?.invalidate(); posTimer = nil
        bridge.playbackEnded(reqID)
    }
}

extension GossamerMedia: UIImagePickerControllerDelegate, UINavigationControllerDelegate {
    func imagePickerController(_ picker: UIImagePickerController,
                              didFinishPickingMediaWithInfo info: [UIImagePickerController.InfoKey: Any]) {
        picker.dismiss(animated: true)
        if let img = info[.originalImage] as? UIImage, let jpeg = img.jpegData(compressionQuality: 0.9) {
            bridge.deliverPhoto(captureReq, data: jpeg)
        } else {
            bridge.failCapture(captureReq, msg: "no image")
        }
    }
    func imagePickerControllerDidCancel(_ picker: UIImagePickerController) {
        picker.dismiss(animated: true)
        bridge.failCapture(captureReq, msg: "cancelled")
    }
}

extension GossamerMedia: AVAudioPlayerDelegate {
    func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        posTimer?.invalidate(); posTimer = nil
        bridge.playbackEnded(playReq)
    }
}
