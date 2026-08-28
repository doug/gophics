// Reference iOS host for the platform capabilities: share sheet, local
// notifications, keychain, file picker and location.
//
// Copy this into your app's iOS project and register it once, after Start:
//
//     let platform = GophicsPlatform(bridge: bridge, present: viewController)
//     bridge.setShareHost(platform)
//     bridge.setNotifyHost(platform)
//     bridge.setSecureHost(platform)
//     bridge.setFileHost(platform)
//     bridge.setLocationHost(platform)
//     bridge.setFilesDir(NSSearchPathForDirectoriesInDomains(
//         .documentDirectory, .userDomainMask, true).first ?? "")
//
// Register only what you use: a capability whose host is not set reads as nil
// in Go, which is how an app knows to hide the affordance.
//
// Every method here runs on the UI thread, which is the Bridge's contract. The
// asynchronous ones answer later on the same thread; the keychain ones answer
// inline, because SecureStorage is synchronous in Go.
import CoreLocation
import Foundation
import Mobile
import Security
import UIKit
import UniformTypeIdentifiers
import UserNotifications

public final class GophicsPlatform: NSObject {
    private let bridge: MobileBridge
    /// The view controller used to present sheets. Weak: the platform host
    /// outlives a pushed controller, and holding it would leak the whole stack.
    private weak var present: UIViewController?

    /// Keychain items are scoped to this service, so a Delete cannot reach
    /// another app's or another library's entries.
    private let service: String

    private var manager: CLLocationManager?
    private var locationRequests: [Int: Bool] = [:] // reqID → isWatch

    public init(bridge: MobileBridge, present: UIViewController, service: String? = nil) {
        self.bridge = bridge
        self.present = present
        self.service = service ?? (Bundle.main.bundleIdentifier ?? "gophics") + ".secure"
        super.init()
    }

    // MARK: - Share

    public func share(_ reqID: Int, title: String?, text: String?, url: String?,
                      fileName: String?, fileData: Data?) {
        var items: [Any] = []
        if let t = text, !t.isEmpty { items.append(t) }
        if let u = url, !u.isEmpty, let parsed = URL(string: u) { items.append(parsed) }
        // A file has to exist on disk for the share sheet to offer the apps that
        // can handle it, so write it to a temporary URL keyed by name.
        if let name = fileName, !name.isEmpty, let data = fileData {
            let tmp = FileManager.default.temporaryDirectory.appendingPathComponent(name)
            do {
                try data.write(to: tmp, options: .atomic)
                items.append(tmp)
            } catch {
                bridge.deliverShareResult(reqID, errMsg: error.localizedDescription)
                return
            }
        }
        guard !items.isEmpty, let host = present else {
            bridge.deliverShareResult(reqID, errMsg: "nothing to share")
            return
        }

        let vc = UIActivityViewController(activityItems: items, applicationActivities: nil)
        if let t = title, !t.isEmpty { vc.setValue(t, forKey: "subject") }
        // iPad presents this as a popover and requires an anchor.
        if let pop = vc.popoverPresentationController {
            pop.sourceView = host.view
            pop.sourceRect = CGRect(x: host.view.bounds.midX, y: host.view.bounds.midY,
                                    width: 0, height: 0)
            pop.permittedArrowDirections = []
        }
        // Dismissal is reported as success: iOS cannot distinguish "cancelled"
        // from "the share extension failed", so treating one as an error shows a
        // failure every time somebody changes their mind.
        vc.completionWithItemsHandler = { [weak self] _, _, _, error in
            self?.bridge.deliverShareResult(reqID, errMsg: error?.localizedDescription ?? "")
        }
        host.present(vc, animated: true)
    }

    // MARK: - Local notifications

    public func authorizeNotify(_ reqID: Int) {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) {
            [weak self] granted, _ in
            // The completion arrives on a background queue; the Bridge is
            // UI-thread only.
            DispatchQueue.main.async {
                self?.bridge.deliverNotifyPermission(reqID, granted: granted)
            }
        }
    }

    public func notify(_ title: String?, body: String?, tag: String?) {
        let content = UNMutableNotificationContent()
        content.title = title ?? ""
        content.body = body ?? ""
        // A non-empty tag replaces the previous notification rather than
        // stacking, which is what the identifier does here.
        let id = (tag?.isEmpty == false) ? tag! : UUID().uuidString
        let req = UNNotificationRequest(identifier: id, content: content, trigger: nil)
        UNUserNotificationCenter.current().add(req)
    }

    // MARK: - Keychain
    //
    // Synchronous by contract. kSecAttrAccessibleAfterFirstUnlock rather than
    // the default: an app that reads a token on launch must work when the phone
    // rebooted and nobody has unlocked it since.

    private func query(_ key: String) -> [String: Any] {
        [kSecClass as String: kSecClassGenericPassword,
         kSecAttrService as String: service,
         kSecAttrAccount as String: key]
    }

    public func secureGet(_ key: String?) -> String {
        guard let key else { return "" }
        var q = query(key)
        q[kSecReturnData as String] = true
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        var out: CFTypeRef?
        guard SecItemCopyMatching(q as CFDictionary, &out) == errSecSuccess,
              let data = out as? Data else { return "" }
        return String(data: data, encoding: .utf8) ?? ""
    }

    public func secureHas(_ key: String?) -> Bool {
        guard let key else { return false }
        var q = query(key)
        q[kSecMatchLimit as String] = kSecMatchLimitOne
        return SecItemCopyMatching(q as CFDictionary, nil) == errSecSuccess
    }

    public func secureSet(_ key: String?, value: String?) -> String {
        guard let key else { return "empty key" }
        let data = Data((value ?? "").utf8)
        let q = query(key)
        let attrs: [String: Any] = [kSecValueData as String: data,
                                    kSecAttrAccessible as String: kSecAttrAccessibleAfterFirstUnlock]
        let status = SecItemUpdate(q as CFDictionary, attrs as CFDictionary)
        if status == errSecSuccess { return "" }
        if status == errSecItemNotFound {
            var add = q
            add[kSecValueData as String] = data
            add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
            let addStatus = SecItemAdd(add as CFDictionary, nil)
            return addStatus == errSecSuccess ? "" : "keychain add failed (\(addStatus))"
        }
        return "keychain update failed (\(status))"
    }

    public func secureDelete(_ key: String?) -> String {
        guard let key else { return "" }
        let status = SecItemDelete(query(key) as CFDictionary)
        // Deleting something that is not there is not an error.
        if status == errSecSuccess || status == errSecItemNotFound { return "" }
        return "keychain delete failed (\(status))"
    }

    // MARK: - Files

    private func contentTypes(_ accept: String?) -> [UTType] {
        guard let accept, !accept.isEmpty else { return [.item] }
        var out: [UTType] = []
        for raw in accept.split(separator: ",") {
            let s = raw.trimmingCharacters(in: .whitespaces)
            if s.hasPrefix(".") {
                if let t = UTType(filenameExtension: String(s.dropFirst())) { out.append(t) }
            } else if let t = UTType(mimeType: s) {
                out.append(t)
            }
        }
        return out.isEmpty ? [.item] : out
    }

    public func pickFiles(_ reqID: Int, accept: String?, multiple: Bool) {
        guard let host = present else {
            bridge.failPick(reqID, msg: "no view controller to present from")
            return
        }
        let vc = UIDocumentPickerViewController(forOpeningContentTypes: contentTypes(accept),
                                                asCopy: true)
        vc.allowsMultipleSelection = multiple
        let d = PickDelegate(reqID: reqID, bridge: bridge) { [weak self] in self?.pickDelegate = nil }
        pickDelegate = d
        vc.delegate = d
        host.present(vc, animated: true)
    }

    public func saveFile(_ reqID: Int, name: String?, accept: String?, data: Data?) {
        guard let host = present else {
            bridge.deliverSaveDone(reqID, errMsg: "no view controller to present from")
            return
        }
        let tmp = FileManager.default.temporaryDirectory
            .appendingPathComponent(name?.isEmpty == false ? name! : "export")
        do {
            try (data ?? Data()).write(to: tmp, options: .atomic)
        } catch {
            bridge.deliverSaveDone(reqID, errMsg: error.localizedDescription)
            return
        }
        let vc = UIDocumentPickerViewController(forExporting: [tmp], asCopy: true)
        let d = SaveDelegate(reqID: reqID, bridge: bridge) { [weak self] in self?.saveDelegate = nil }
        saveDelegate = d
        vc.delegate = d
        host.present(vc, animated: true)
    }

    // UIKit holds its delegate weakly, so these keep the in-flight one alive.
    private var pickDelegate: PickDelegate?
    private var saveDelegate: SaveDelegate?

    // MARK: - Location

    public func startLocation(_ reqID: Int, watch: Bool) {
        let m = manager ?? {
            let m = CLLocationManager()
            m.delegate = self
            manager = m
            return m
        }()
        locationRequests[reqID] = watch
        switch m.authorizationStatus {
        case .notDetermined:
            m.requestWhenInUseAuthorization() // the fix follows the grant
        case .denied, .restricted:
            locationRequests[reqID] = nil
            bridge.failLocation(reqID, msg: "location permission denied")
            return
        default:
            break
        }
        if watch {
            m.startUpdatingLocation()
        } else {
            m.requestLocation()
        }
    }

    public func stopLocation(_ reqID: Int) {
        locationRequests[reqID] = nil
        if locationRequests.values.contains(true) { return }
        manager?.stopUpdatingLocation()
    }
}

extension GophicsPlatform: CLLocationManagerDelegate {
    public func locationManager(_ m: CLLocationManager, didUpdateLocations locs: [CLLocation]) {
        guard let l = locs.last else { return }
        for (reqID, _) in locationRequests {
            bridge.deliverLocation(reqID,
                                   lat: l.coordinate.latitude,
                                   lon: l.coordinate.longitude,
                                   accuracy: l.horizontalAccuracy)
        }
        // One-shot requests are cleared by the Go side, which calls back into
        // stopLocation; nothing to prune here.
    }

    public func locationManager(_ m: CLLocationManager, didFailWithError error: Error) {
        for (reqID, _) in locationRequests {
            bridge.failLocation(reqID, msg: error.localizedDescription)
        }
        locationRequests.removeAll()
    }

    public func locationManagerDidChangeAuthorization(_ m: CLLocationManager) {
        guard m.authorizationStatus == .denied || m.authorizationStatus == .restricted else { return }
        for (reqID, _) in locationRequests {
            bridge.failLocation(reqID, msg: "location permission denied")
        }
        locationRequests.removeAll()
    }
}

/// Document-picker delegate for opening. Separate objects per request keep two
/// concurrent pickers from answering each other's reqID.
private final class PickDelegate: NSObject, UIDocumentPickerDelegate {
    private let reqID: Int
    private let bridge: MobileBridge
    private let finish: () -> Void

    init(reqID: Int, bridge: MobileBridge, finish: @escaping () -> Void) {
        self.reqID = reqID
        self.bridge = bridge
        self.finish = finish
    }

    func documentPicker(_ c: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        for url in urls {
            // asCopy: true still hands back a security-scoped URL on some paths,
            // and the bytes are only readable inside the access pair.
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            guard let data = try? Data(contentsOf: url) else { continue }
            bridge.deliverPickedFile(reqID, name: url.lastPathComponent, data: data)
        }
        bridge.deliverPickedDone(reqID)
        finish()
    }

    func documentPickerWasCancelled(_ c: UIDocumentPickerViewController) {
        // Cancel is an empty selection, not a failure.
        bridge.deliverPickedDone(reqID)
        finish()
    }
}

/// Document-picker delegate for exporting.
private final class SaveDelegate: NSObject, UIDocumentPickerDelegate {
    private let reqID: Int
    private let bridge: MobileBridge
    private let finish: () -> Void

    init(reqID: Int, bridge: MobileBridge, finish: @escaping () -> Void) {
        self.reqID = reqID
        self.bridge = bridge
        self.finish = finish
    }

    func documentPicker(_ c: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        bridge.deliverSaveDone(reqID, errMsg: "")
        finish()
    }

    func documentPickerWasCancelled(_ c: UIDocumentPickerViewController) {
        bridge.deliverSaveDone(reqID, errMsg: "")
        finish()
    }
}
